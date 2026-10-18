import { useEffect, useState } from "react";

function sameLocalDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

/**
 * The viewer's current date, renewed at local midnight and when the page becomes visible again,
 * since a timer can fire late after the device sleeps.
 */
export function useLocalToday(): Date {
  const [today, setToday] = useState(() => new Date());
  useEffect(() => {
    function renew() {
      const now = new Date();
      if (!sameLocalDay(now, today)) setToday(now);
    }
    const midnight = new Date(today.getFullYear(), today.getMonth(), today.getDate() + 1);
    // Always take a new date here: one that fires early still schedules the next check.
    const timer = setTimeout(() => setToday(new Date()), midnight.getTime() - Date.now());
    function onVisible() {
      if (document.visibilityState === "visible") renew();
    }
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [today]);
  return today;
}
