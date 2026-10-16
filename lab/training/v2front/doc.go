// Package v2front is front end V. It lives outside policynet so that only
// the SpellBench bridge tools, which import it, link the protocol-v2 wire
// types; seat and gorged link policynet without them.
//
// Bridge build tag: every other file in this package carries
// //go:build spellbench_bridge. The SpellBench protocol v1/v2 bridge is a
// parked program, so it is excluded from go test ./..., make test and the
// pipeline gates; build or test it with -tags spellbench_bridge
// (make test-spellbench-bridge). This file stays untagged so go list ./...
// and go vet ./... still see the package.
package v2front
