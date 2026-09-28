# Management table pagination

## Goal

Add one consistent client-side pagination pattern to every tabular management surface instead of letting growing Provider, Tool, Profile, Server, Policy, Credential, and Audit inventories expand a page without bounds.

The Overview recent-invocations list uses the same pagination control because it is presented as a row-oriented history surface even though it is not implemented with the shared Table primitive.

## Scope

Pagination applies to:

- Providers;
- Catalog Tools;
- Profiles;
- Servers;
- Policy Rules;
- Credentials;
- Audit invocation history;
- Overview recent invocations.

Configuration/editor lists inside focused dialogs are not tables and remain scroll/progressive-disclosure surfaces.

## Interaction contract

1. Every non-empty paginated surface shows a compact footer with:
   - visible row range and total rows;
   - rows-per-page selector;
   - current page and page count;
   - Previous / Next controls.
2. Default management-table page size is 10 rows.
3. Overview recent invocations defaults to 5 rows per page to preserve dashboard density.
4. Page-size choices are 10, 25, and 50 for management tables; Overview also allows 5.
5. Changing page size returns to the first page.
6. When a data mutation shrinks a collection, the current page is clamped to the last valid page.
7. Catalog browse/search replaces the result set and returns pagination to page 1.
8. Empty states do not render meaningless pagination controls.

## Data-boundary decision

This change paginates the collections already returned by the management client. It does not redefine public list/search contracts or introduce offset/cursor semantics.

Catalog discovery remains intentionally bounded by the existing browse/search limits, and Audit remains bounded by its existing list limit. Pagination therefore improves management-page usability without coupling the UI refinement to a new backend pagination protocol.

A future backend/cursor phase can replace the internal collection source without changing the shared pagination footer contract.

## Implementation shape

- Add a shared `usePagination` hook in `packages/app/src/shared`.
- Add a shared `TablePagination` footer component in the same module.
- Keep page state local to each page/surface.
- Slice only at render time so canonical client data remains available to dialogs, selectors, counts, and mutations.
- Add focused tests for range calculation, navigation, page-size changes, and page clamping.
- Extend management-page tests to verify paginated rendering on representative pages.

## Done when

- every Table-backed management list uses the shared pagination helper;
- Overview recent invocations is paginated consistently;
- no page renders more than its selected page size;
- pagination remains correct after refresh, mutation, and Catalog query replacement;
- typecheck, lint, and frontend tests pass;
- unrelated working-tree changes remain untouched.
