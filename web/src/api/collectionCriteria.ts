import type { ListFilterParams } from "./client";

export type CollectionCriteria = Pick<
  ListFilterParams,
  "watch" | "playable" | "favorite" | "sort" | "seed"
>;

export function setCollectionCriteria(
  params: URLSearchParams,
  criteria?: CollectionCriteria,
) {
  if (criteria?.watch && criteria.watch !== "all") params.set("watch", criteria.watch);
  if (criteria?.playable) params.set("playable", "true");
  if (criteria?.favorite) params.set("favorite", "true");
  if (criteria?.sort) params.set("sort", criteria.sort);
  if (criteria?.sort === "random" && criteria.seed !== undefined)
    params.set("seed", String(criteria.seed));
}
