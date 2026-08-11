// Package agenticstream anchors the project root so that all Go tooling
// (build, vet, test, lint) finds at least one package to operate on
// while the application is still being scaffolded. Once real packages
// exist under cmd/ and internal/, this file can be deleted or
// repurposed to host shared types at the module root.
//
// See AGENTS.md for the canonical project definition.
package agenticstream
