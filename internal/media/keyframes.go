package media

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Sampling is optional: bound the extra import work independently of the main
// metadata probe. Packets are demuxed, never decoded. A partial/failed run is unknown.
const keyframeProbeTimeout = 500 * time.Millisecond
const keyframeOutputLimit = 256 << 10

// ProbeKeyframes samples the first 16 seconds of the first non-attached video.
// The final second lets us observe a gap strictly greater than CopySeekAllowance.
func ProbeKeyframes(ctx context.Context, path string) domain.KeyframeProbe {
	return probeKeyframes(ctx, path, keyframeProbeTimeout)
}

func probeKeyframes(ctx context.Context, path string, timeout time.Duration) domain.KeyframeProbe {
	result := domain.KeyframeProbe{Version: domain.KeyframeProbeVersion, Spacing: domain.KeyframesUnknown}
	before, err := os.Stat(path)
	if err != nil {
		return result
	}
	result.Source = domain.FileStampOf(before)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := command(probeCtx, probeCommand,
		"-v", "error", "-select_streams", "V:0", "-read_intervals", "%+16",
		"-show_packets", "-show_entries", "packet=pts_time,flags", "-of", "compact=p=0:nk=0", "--", path)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return result
	}
	if err := cmd.Start(); err != nil {
		return result
	}
	data, readErr := io.ReadAll(io.LimitReader(output, keyframeOutputLimit+1))
	if readErr != nil || len(data) > keyframeOutputLimit {
		cancel()
	}
	runErr := cmd.Wait()
	after, statErr := os.Stat(path)
	if statErr != nil || domain.FileStampOf(after) != result.Source {
		// A zero version also prevents the background writer from saving this result.
		return domain.KeyframeProbe{}
	}
	if readErr == nil && runErr == nil && probeCtx.Err() == nil && len(data) <= keyframeOutputLimit {
		result.Spacing = classifyKeyframePackets(string(data))
	}
	return result
}

func classifyKeyframePackets(output string) domain.KeyframeSpacing {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var keyTime float64
	haveKey, haveInterval, sparse := false, false, false
	for scanner.Scan() {
		pts, key, ok := keyframePacket(scanner.Text())
		if !ok {
			return domain.KeyframesUnknown
		}
		if haveKey && pts-keyTime > domain.CopySeekAllowance.Seconds() {
			sparse = true
		}
		if key {
			if haveKey && pts <= keyTime {
				return domain.KeyframesUnknown
			}
			haveInterval = haveInterval || haveKey
			keyTime, haveKey = pts, true
		}
	}
	if scanner.Err() != nil {
		return domain.KeyframesUnknown
	}
	if sparse {
		return domain.KeyframesSparse
	}
	if haveInterval {
		return domain.KeyframesShort
	}
	return domain.KeyframesUnknown
}

func keyframePacket(line string) (float64, bool, bool) {
	var pts float64
	var key, haveTime, haveFlags bool
	for _, field := range strings.Split(line, "|") {
		name, value, _ := strings.Cut(field, "=")
		switch name {
		case "pts_time":
			var err error
			pts, err = strconv.ParseFloat(value, 64)
			haveTime = err == nil && !math.IsNaN(pts) && !math.IsInf(pts, 0)
		case "flags":
			key, haveFlags = strings.Contains(value, "K"), value != ""
		}
	}
	return pts, key, haveTime && haveFlags
}
