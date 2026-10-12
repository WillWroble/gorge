// Package gpuscorer is internal/gamepool's GPU scorer: a flat-net forward run
// on the device through the CUDA Driver API, reached from pure Go with purego
// (dlopen/dlsym, no cgo).
//
// It is its own nested module (experimental/gamepool) rather than part of the
// root module because purego is a third-party dependency and the root go.mod
// carries no require lines (docs/agents/invariants.md section 7, enforced by
// internal/archtest TestRootModuleHasNoRequires). The pool scheduler and the
// MageZero encoders it feeds from stay in the root module's internal/gamepool;
// this module imports them through a replace of the root module.
//
// Every real file is behind -tags 'gamepool gamepool_gpu'; this file keeps the
// package buildable without them.
package gpuscorer
