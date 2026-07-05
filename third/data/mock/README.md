# Mock Data

`mock_data.json` is local seed data for the `ThirdBusinessService` RPC surface.

## Mapping

- `users`: base user profiles for scenario setup.
- `products`: full product catalog.
- `recommendations`: maps `user_id` to recommended `product_id` values for `RecommendProducts`.
- `im_conversations`: merchant-user IM conversations. Used by `ListIMConversations`,
  `GetIMConversationMessages`, and `SendIMMessage`.
- `user_selection_carts`: current product IDs in each user's selection cart.
- `free_sample_applications`: current free-sample applications per user. The
  seed starts with empty arrays; write APIs append runtime records.
- `buy_sample_orders`: current paid-sample orders per user.
- `product_rules`: product-level rules used by `CheckFreeSample`, `AddSelectionCart`,
  `ApplyFreeSample`, and `BuySample`.
- `GetFulfillmentStatus` is derived at query time from `user_selection_carts`,
  `free_sample_applications`, and `buy_sample_orders`; it is not pre-baked in
  the seed file.

At process startup, this seed file is copied to `data/runtime/mock_data.json`.
All write APIs mutate the runtime copy, so a new process starts from the same
seed state again.

## Fulfillment Status Design

`stage` is the business phase:

- `CARTING`: product has entered or failed to enter the selection cart flow.
- `FREE_SAMPLE`: free sample application / review / shipping flow.
- `PAID_SAMPLE`: paid sample order flow.
- `DELIVERY_FULFILLMENT`: post-sample delivery and content verification flow.

`status` is the state inside that phase:

- `NOT_STARTED`
- `PROCESSING`
- `SUCCESS`
- `FAILED`
- `CANCELLED`
