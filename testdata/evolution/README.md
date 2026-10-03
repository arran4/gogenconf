# Enrichment in four files

`v1.conf` and `v2.conf` are seeded documents from the two small schemas in
`evolution_test.go`. `customized.conf` is the user's v1 document.

The registered v1 → v2 migration advances version metadata; enrichment then
refreshes managed endpoint documentation and adds `retries 3`. Compare
`enriched.conf`: the private endpoint, timeout override, user comments, and
unknown extension survive. The changed endpoint default is not applied over
the user's value. A parse/re-enrich/format pass must match the same golden.
