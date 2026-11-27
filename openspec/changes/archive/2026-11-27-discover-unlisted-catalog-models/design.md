## Decisions

Parse the catalog once, then keep configured rows and use the existing bounded, no-redirect inventory path for opted-in providers. Share the health and inventory admission limits; on contention or listing failure return the configured rows and any safe inventory already collected. Bound discovery to two seconds and the existing 1,024-ID budget including aliases. Do not cache or gate routing on inventory. Invokers see eligible selectors, not upstream errors, keys, or other inventory fields. The pinned Vercel Gateway client continues to read the same `/config` model rows.

Remove the alternate blocklist UI state. Writers who need to set mode and exclusions together can use one catalog API update.
