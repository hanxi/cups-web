# Interface translations

`en.json` and `zh-CN.json` contain application-owned display text. Use descriptive
keys and named parameters with `useI18n().t()`; keep translated option arrays and
table headers in `computed()` so switching languages never resets a form.

Language is a browser preference (`cups-web.locale` in localStorage), not an
account or server setting. A saved supported choice wins over the first supported
entry in `navigator.languages`; otherwise English is used. Automatic detection
never writes storage. All English and Chinese regional tags select English and
Simplified Chinese respectively. The header selector persists only explicit
choices and still works if storage is unavailable.

`api-errors.json` matches known application-owned API messages in the UI only.
New error messages need a matching rule and translations. Do not translate
protocol values, user-entered text, document content, filenames, printer names,
vendor-provided PPD labels or unknown external diagnostics. Raw diagnostic logs
are intentionally preserved. `printerChoiceLabel()` handles app-added borderless
suffixes and standard IPP quality labels without modifying the submitted values.

Run `npm test` for preference, reactive component, API error and catalog checks,
and `npm run build` for the full frontend build.

## Translation provenance

A substantial portion of the initial English and Chinese wording and API error
patterns was adapted from [Alcedema/cups-web](https://github.com/Alcedema/cups-web)
at commit `172eab8046d514ab60a49f2efff843e1f4307853` (MIT), based on
[hanxi/cups-web](https://github.com/hanxi/cups-web). Only translation data was
adapted; the current upstream components and backend remain the implementation
baseline. Account/server language policy, account features and fork branding were
not adopted. New upstream UI strings were translated separately, and keys were
renamed to descriptive identifiers.

The source's license notice is reproduced below:

MIT License

Copyright (c) 2025 涵曦

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
