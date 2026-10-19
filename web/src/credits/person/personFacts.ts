import type { Person } from "../../api/credits";
import { t } from "../../i18n";

export type PersonProfile = Person["profile"];

/** Year, month and day, with month and day counted from 1. */
type Day = readonly [number, number, number];

interface DayBounds {
  first: Day;
  last: Day;
}

const datePattern = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$/;

function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

/** The first and last day a date at its own precision (YYYY, YYYY-MM or YYYY-MM-DD) allows. */
export function dateBounds(date: string): DayBounds | null {
  const match = datePattern.exec(date);
  if (match === null) return null;
  const year = Number(match[1]);
  if (match[2] === undefined) return { first: [year, 1, 1], last: [year, 12, 31] };
  const month = Number(match[2]);
  if (match[3] === undefined) {
    return { first: [year, month, 1], last: [year, month, daysInMonth(year, month)] };
  }
  const day: Day = [year, month, Number(match[3])];
  return { first: day, last: day };
}

function ageOn(birth: Day, on: Day): number {
  const beforeBirthday = on[1] < birth[1] || (on[1] === birth[1] && on[2] < birth[2]);
  return on[0] - birth[0] - (beforeBirthday ? 1 : 0);
}

export interface AgeRange {
  low: number;
  high: number;
}

/**
 * The youngest and oldest ages the dates allow, on the death date when there is one and on the
 * viewer's local date otherwise; equal bounds mean an exact age.
 */
export function ageRange(
  birthDate: string,
  deathDate: string | undefined,
  today: Date,
): AgeRange | null {
  const birth = dateBounds(birthDate);
  if (birth === null) return null;
  const now: Day = [today.getFullYear(), today.getMonth() + 1, today.getDate()];
  const on = deathDate === undefined ? { first: now, last: now } : dateBounds(deathDate);
  if (on === null) return null;
  const low = Math.max(0, ageOn(birth.last, on.first));
  return { low, high: Math.max(low, ageOn(birth.first, on.last)) };
}

export interface PersonFact {
  id: string;
  term: string;
  value: string;
}

export interface PersonFactGroup {
  id: "profile" | "measurements" | "career";
  heading: string;
  facts: PersonFact[];
  /** Only the career group carries the bio, shown after its facts. */
  bio?: string;
}

function lifeFacts(profile: PersonProfile, today: Date): PersonFact[] {
  const text = t.credits.personPage;
  const { birthDate, deathDate } = profile;
  const range = birthDate === undefined ? null : ageRange(birthDate, deathDate, today);
  const age = range === null ? null : text.ageRange(range.low, range.high);
  const facts: PersonFact[] = [];
  if (birthDate !== undefined) {
    const value =
      age === null || deathDate !== undefined
        ? birthDate
        : text.dateWithAge(birthDate, text.age(age));
    facts.push({ id: "born", term: text.facts.born, value });
  }
  if (deathDate !== undefined) {
    const value =
      age === null ? deathDate : text.dateWithAge(deathDate, text.agedAtDeath(age));
    facts.push({ id: "died", term: text.facts.died, value });
  }
  return facts;
}

function profileFacts(profile: PersonProfile, today: Date): PersonFact[] {
  const text = t.credits.personPage;
  const facts = lifeFacts(profile, today);
  if (profile.gender !== undefined) {
    facts.push({
      id: "gender",
      term: text.facts.gender,
      value: text.genders[profile.gender],
    });
  }
  if (profile.birthplace !== undefined) {
    facts.push({
      id: "birthplace",
      term: text.facts.birthplace,
      value: profile.birthplace,
    });
  }
  if (profile.bloodType !== undefined) {
    facts.push({ id: "bloodType", term: text.facts.bloodType, value: profile.bloodType });
  }
  return facts;
}

function measurementFacts(profile: PersonProfile): PersonFact[] {
  const text = t.credits.personPage;
  const facts: PersonFact[] = [];
  if (profile.heightCm !== undefined) {
    facts.push({
      id: "height",
      term: text.facts.height,
      value: text.centimetres(profile.heightCm),
    });
  }
  const sizes = [profile.bustCm, profile.waistCm, profile.hipsCm];
  if (sizes.some((size) => size !== undefined)) {
    facts.push({
      id: "measurements",
      term: text.facts.measurements,
      value: text.measurementsValue(
        sizes.map((size) =>
          size === undefined ? text.missingMeasurement : String(size),
        ),
      ),
    });
  }
  if (profile.cup !== undefined) {
    facts.push({ id: "cup", term: text.facts.cup, value: profile.cup });
  }
  return facts;
}

function careerValue(from: number | undefined, until: number | undefined): string | null {
  const text = t.credits.personPage;
  if (from !== undefined && until !== undefined) return text.careerRange(from, until);
  if (from !== undefined) return text.careerSince(from);
  if (until !== undefined) return text.careerUntil(until);
  return null;
}

/** The page's fact groups in order, each with only its filled rows; empty groups are left out. */
export function personFactGroups(profile: PersonProfile, today: Date): PersonFactGroup[] {
  const text = t.credits.personPage;
  const career = careerValue(profile.careerStartYear, profile.careerEndYear);
  const groups: PersonFactGroup[] = [
    { id: "profile", heading: text.groups.profile, facts: profileFacts(profile, today) },
    {
      id: "measurements",
      heading: text.groups.measurements,
      facts: measurementFacts(profile),
    },
    {
      id: "career",
      heading: text.groups.career,
      facts:
        career === null ? [] : [{ id: "career", term: text.facts.career, value: career }],
      bio: profile.bio,
    },
  ];
  return groups.filter((group) => group.facts.length > 0 || group.bio !== undefined);
}
