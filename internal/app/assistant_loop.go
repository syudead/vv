package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AssistantTurnStore persists turns; every change publishes AssistantChanged after commit, and a running turn moves
// to a finished status once.
type AssistantTurnStore interface {
	// CreateTurn returns the running turn with domain.ErrAssistantBusy while one runs.
	CreateTurn(ctx context.Context, in domain.AssistantTurnInput, now time.Time) (domain.AssistantTurn, error)
	// Turn returns domain.ErrNotFound for a missing or expired turn.
	Turn(ctx context.Context, id int64) (domain.AssistantTurn, error)
	// RunningTurn returns domain.ErrNoJob when no turn runs.
	RunningTurn(ctx context.Context) (domain.AssistantTurn, error)
	ActiveTurns(ctx context.Context, since time.Time) ([]domain.AssistantTurn, error)
	RecordTurnProgress(ctx context.Context, id int64, progress []domain.AssistantProgressItem) error
	// FinishTurn returns domain.ErrAssistantTurnNotRunning for a turn that already finished.
	FinishTurn(ctx context.Context, turn domain.AssistantTurn, now time.Time) error
	RecoverTurns(ctx context.Context, now time.Time) (int, error)
	ExpireTurns(ctx context.Context, before time.Time) (int, error)
}

// StartTurn stores the request as a running turn and wakes the turn worker; while another turn runs it returns
// that turn with domain.ErrAssistantBusy.
func (a *Assistant) StartTurn(ctx context.Context, in domain.AssistantTurnInput) (domain.AssistantTurn, error) {
	if a.turns == nil || a.chat == nil || a.vocabulary == nil {
		return domain.AssistantTurn{}, errors.New("the assistant cannot run requests: it is not configured")
	}
	turn, err := a.turns.CreateTurn(ctx, in, a.now())
	if err != nil {
		return turn, err
	}
	a.wakeTurns()
	return turn, nil
}

// Turn returns domain.ErrNotFound for a missing or expired turn.
func (a *Assistant) Turn(ctx context.Context, id int64) (domain.AssistantTurn, error) {
	return a.turns.Turn(ctx, id)
}

// ActiveTurns lists the running turn and the turns finished within the last hour, newest first.
func (a *Assistant) ActiveTurns(ctx context.Context) ([]domain.AssistantTurn, error) {
	return a.turns.ActiveTurns(ctx, a.now().Add(-domain.AssistantActiveTurnWindow))
}

func (a *Assistant) wakeTurns() {
	select {
	case a.turnWake <- struct{}{}:
	default:
	}
}

func (a *Assistant) recoverTurns(ctx context.Context) error {
	if a.turns == nil {
		return nil
	}
	recovered, err := a.turns.RecoverTurns(ctx, a.now())
	if err != nil {
		return err
	}
	if recovered > 0 {
		a.logger.Info("interrupted the assistant turns of the previous run", slog.Int("count", recovered))
	}
	return nil
}

// RunTurns runs the running turn's loop, one turn at a time, and deletes turns finished more than 24 hours ago. A
// turn cut short by shutdown stays running for Recover to mark interrupted at the next start.
func (a *Assistant) RunTurns(ctx context.Context) {
	if a.turns == nil {
		return
	}
	var ran int64
	for ctx.Err() == nil {
		if _, err := a.turns.ExpireTurns(ctx, a.now().Add(-domain.AssistantTurnTTL)); err != nil && ctx.Err() == nil {
			a.logger.Error("cannot delete the expired assistant turns", slog.Any("error", err))
		}
		runner := a.currentRunner()
		if runner == nil {
			a.waitTurns(ctx, nil)
			continue
		}
		turn, err := a.turns.RunningTurn(ctx)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			a.waitTurns(ctx, time.After(assistantExpiryInterval))
		case err != nil:
			if ctx.Err() == nil {
				a.logger.Error("cannot read the running assistant turn", slog.Any("error", err))
			}
			a.waitTurns(ctx, time.After(assistantRetryDelay))
		case turn.ID == ran:
			// Finishing it failed once; end it rather than run the request twice.
			turn.Status = domain.AssistantTurnInterrupted
			a.finishTurn(ctx, turn)
			a.waitTurns(ctx, time.After(assistantRetryDelay))
		default:
			ran = turn.ID
			a.runTurn(ctx, runner, turn)
		}
	}
}

func (a *Assistant) waitTurns(ctx context.Context, timer <-chan time.Time) {
	select {
	case <-ctx.Done():
	case <-a.turnWake:
	case <-timer:
	}
}

// runTurn runs the loop and stores how the turn ended; a storage failure inside the loop ends it interrupted.
func (a *Assistant) runTurn(ctx context.Context, runner ActionRunner, turn domain.AssistantTurn) {
	loop := newTurnLoop(a, runner, turn)
	err := loop.run(ctx)
	if err == nil {
		err = loop.propose(ctx)
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		a.logger.Error("the assistant turn failed", slog.Int64("turn", turn.ID), slog.Any("error", err))
		loop.turn.Status = domain.AssistantTurnInterrupted
	}
	a.finishTurn(ctx, loop.turn)
}

func (a *Assistant) finishTurn(ctx context.Context, turn domain.AssistantTurn) {
	if err := a.turns.FinishTurn(ctx, turn, a.now()); err != nil && ctx.Err() == nil {
		a.logger.Error("cannot finish the assistant turn", slog.Int64("turn", turn.ID), slog.Any("error", err))
	}
}

// turnLoop is one turn's state while it runs; the transcript lives only here.
type turnLoop struct {
	a        *Assistant
	runner   ActionRunner
	turn     domain.AssistantTurn
	deadline time.Time
	messages []domain.ChatMessage
	tools    *turnTools
	// evidence holds what the grounding check accepts: the request, the screen, today's date and every tool result.
	evidence   strings.Builder
	modelCalls int
	toolCalls  int
	corrected  bool
	// prompted marks a turn whose empty reply already received its correction round.
	prompted bool
	// usage is how the provider ended the last reply.
	usage domain.ChatUsage
	// clef and vision say which image tools the turn offers; judged marks a turn that received Clef's answers.
	clef, vision, judged bool
	// attached and attachedNames hold the images view_images read this round, for the message after the results.
	attached      [][]byte
	attachedNames []string
	// excluded is the first excluded operation the model asked for, and acted whether any other tool ran.
	excluded *domain.AssistantExcludedRef
	acted    bool
	writes   turnWrites
}

func newTurnLoop(a *Assistant, runner ActionRunner, turn domain.AssistantTurn) *turnLoop {
	l := &turnLoop{
		a: a, runner: runner, turn: turn, deadline: turn.StartedAt.Add(domain.AssistantTurnBudget),
		tools: newTurnTools(runner.Tools()),
	}
	l.tools.builtins = a.builtinDefinitions()
	return l
}

// run returns an error only for a storage failure; every other ending is the turn's status.
func (l *turnLoop) run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, l.deadline.Sub(l.a.now()))
	defer cancel()
	system, err := l.systemMessage(ctx)
	if err != nil {
		return err
	}
	l.imageTools(ctx)
	l.messages = []domain.ChatMessage{
		{Role: domain.ChatRoleSystem, Content: system},
		{Role: domain.ChatRoleUser, Content: "<request>" + dataText.Replace(l.turn.Request) + "</request>"},
	}
	for {
		if l.modelCalls >= domain.AssistantModelCallMax {
			l.stop(domain.AssistantStopStepLimit)
			return nil
		}
		if l.expired(ctx) {
			l.stop(domain.AssistantStopTimeout)
			return nil
		}
		l.modelCalls++
		tools := append(l.tools.definitions(), l.imageDefinitions()...)
		l.fitContext(tools)
		reply, err := l.a.chat.Chat(ctx, domain.ChatRequest{Messages: l.messages, Tools: tools})
		if err != nil {
			l.stop(l.chatStop(ctx, err))
			if l.turn.Stop.Reason == domain.AssistantStopModel {
				l.turn.Stop.Detail = err.Error()
			}
			return nil
		}
		l.usage = reply.Usage
		if len(reply.ToolCalls) == 0 {
			if done, err := l.answer(ctx, reply.Text); done || err != nil {
				return err
			}
			continue
		}
		if done, err := l.callTools(ctx, reply); done || err != nil {
			return err
		}
	}
}

// expired reports whether the budget is spent, by the context or by the assistant's clock.
func (l *turnLoop) expired(ctx context.Context) bool {
	return ctx.Err() != nil || !l.a.now().Before(l.deadline)
}

func (l *turnLoop) stop(reason domain.AssistantStopReason) {
	l.turn.Status, l.turn.Stop = domain.AssistantTurnStopped, &domain.AssistantTurnStop{Reason: reason}
	l.turn.Answer = ""
}

// chatStop maps a model call failure to the reason the panel shows.
func (l *turnLoop) chatStop(ctx context.Context, err error) domain.AssistantStopReason {
	switch {
	case l.expired(ctx), errors.Is(err, domain.ErrOllamaTimeout), errors.Is(err, context.DeadlineExceeded):
		return domain.AssistantStopTimeout
	case errors.Is(err, domain.ErrOllamaUnreachable), errors.Is(err, domain.ErrNotOllama):
		return domain.AssistantStopConnection
	}
	l.a.logger.Info("the generation model failed", slog.Int64("turn", l.turn.ID), slog.Any("error", err))
	return domain.AssistantStopModel
}

// callTools runs the reply's calls in order and reports whether the turn ended.
func (l *turnLoop) callTools(ctx context.Context, reply domain.ChatReply) (bool, error) {
	l.messages = append(l.messages, domain.ChatMessage{
		Role: domain.ChatRoleAssistant, Content: reply.Text, ToolCalls: reply.ToolCalls,
	})
	for _, call := range reply.ToolCalls {
		if l.toolCalls >= domain.AssistantToolCallMax {
			l.stop(domain.AssistantStopStepLimit)
			return true, nil
		}
		l.toolCalls++
		content, err := l.call(ctx, call)
		if err != nil || l.turn.Status == domain.AssistantTurnClarify {
			return true, err
		}
		if l.expired(ctx) {
			l.stop(domain.AssistantStopTimeout)
			return true, nil
		}
		l.evidence.WriteString(content + "\n")
		l.messages = append(l.messages, domain.ChatMessage{Role: domain.ChatRoleTool, ToolName: call.Name, Content: content})
	}
	l.attachImages()
	return false, nil
}

// answer applies the grounding check: the first failure is a correction round, the second stops the turn.
func (l *turnLoop) answer(ctx context.Context, text string) (bool, error) {
	if l.excluded != nil && !l.acted && l.turn.ProposalID == 0 {
		// The panel answers the excluded operation's reason and page in place of the model's words.
		l.stop(domain.AssistantStopModel)
		l.turn.Stop.Excluded = l.excluded
		return true, nil
	}
	if strings.TrimSpace(text) == "" {
		return l.emptyReply(), nil
	}
	names, err := l.a.vocabulary.NamesIn(ctx, text)
	if err != nil {
		return true, err
	}
	ungrounded := domain.AssistantUngrounded(text, l.evidence.String(), names)
	if len(ungrounded) == 0 {
		l.turn.Status, l.turn.Answer, l.turn.Judged = domain.AssistantTurnDone, text, l.judged
		return true, nil
	}
	if l.corrected {
		l.stop(domain.AssistantStopUngrounded)
		return true, nil
	}
	l.corrected = true
	l.messages = append(l.messages,
		domain.ChatMessage{Role: domain.ChatRoleAssistant, Content: text},
		domain.ChatMessage{Role: domain.ChatRoleUser, Content: fmt.Sprintf(
			"No tool result supports these items of your answer: %s. Check them with the tools, or leave them out, "+
				"and answer again.", strings.Join(ungrounded, ", "))},
	)
	return false, nil
}

// emptyAnswerCorrection asks again after a reply with neither text nor tool calls, which some models send when the
// call stays in their reasoning.
const emptyAnswerCorrection = "Your reply was empty. Answer the request in text, or call a tool."

// emptyReply gives a reply with neither text nor tool calls one correction round, and stops the turn as no_answer on
// the second; it reports whether the turn ended.
func (l *turnLoop) emptyReply() bool {
	if l.prompted {
		l.stop(domain.AssistantStopNoAnswer)
		l.turn.Stop.Detail = emptyReplyDetail(l.usage)
		return true
	}
	l.prompted = true
	l.messages = append(l.messages, domain.ChatMessage{Role: domain.ChatRoleUser, Content: emptyAnswerCorrection})
	return false
}

// emptyReplyDetail says how the provider ended the last empty reply; "length" means the context or the output limit
// filled up, often with reasoning.
func emptyReplyDetail(u domain.ChatUsage) string {
	if u == (domain.ChatUsage{}) {
		return ""
	}
	return fmt.Sprintf("done_reason %q, prompt %d tokens, output %d tokens, reasoning %d characters",
		u.DoneReason, u.PromptTokens, u.OutputTokens, u.ThinkingChars)
}
