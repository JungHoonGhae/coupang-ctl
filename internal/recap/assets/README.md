# Recap assets

- `type-roster-v2.svg` is the active 4 by 4 character sheet. It is a synthetic,
  non-identifying pencil drawing generated for `coupangctl` and traced into
  editable vector paths. The recap crops the correct cell for each v4 type and
  embeds the SVG, so no network asset is required.
- `recap-collage.webp` and `type-roster.webp` are retained legacy studies and
  are not embedded by the current renderer.
- The artwork contains no customer order payloads, product names, identifiers,
  or Coupang branding.
- `Gaegu-Bold.ttf` comes from the Google Fonts `gaegu` family and is distributed
  under the SIL Open Font License 1.1. The license text is preserved in
  `Gaegu-OFL.txt`.

The renderer embeds these assets into the standalone recap. They must remain
presentation-only dependencies and must not enter the typed analytics core.
