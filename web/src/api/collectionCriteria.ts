import type { ListFilterParams } from "./client";

export type CollectionCriteria = Pick<ListFilterParams, "favorite" | "sort" | "seed">;

export function setCollectionCriteria(
  params: URLSearchParams,
  criteria?: CollectionCriteria,
) {
  if (criteria?.favorite) params.set("favorite", "true");
  if (criteria?.sort) params.set("sort", criteria.sort);
  if (criteria?.sort === "random" && criteria.seed !== undefined)
    params.set("seed", String(criteria.seed));
}
