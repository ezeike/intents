# testdata

`clinc150_subset.jsonl` — a subset of the CLINC150 "plus" test split
([clinc/clinc_oos](https://huggingface.co/datasets/clinc/clinc_oos); Larson et al., 2019,
"An Evaluation Dataset for Intent Classification and Out-of-Scope Prediction"; CC BY 3.0).

Contents, in original test-split order:
- 45 intents × 30 utterances from three domains: banking, credit_cards, travel
  (see `clincDomains` in `clinc_helpers_test.go`)
- the first 300 `oos` (out-of-scope) utterances

Rows are `{"text": "...", "intent": "<name>"}`. Splits are fixed by position (see `clincSplit`):
per intent the first 10 rows are dev and the next 20 are test; for `oos`, 100 dev / 200 test.
Tune anything (thresholds, wording) on dev only; report on test.

Known label noise, left as-is: e.g. "repeat what the weather will be like" is labelled `transfer`.
