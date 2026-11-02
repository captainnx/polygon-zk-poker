# Vendored Noir dependency

`keccak256-0.1.2/` is the unmodified official [noir-lang/keccak256 v0.1.2](https://github.com/noir-lang/keccak256/tree/v0.1.2) archive. The upstream tag does not contain a LICENSE file; this repository does not add or assert an upstream license grant.

Archive SHA256: `6a9b8bbbc146f80b4ccda1304df492a36118cd7b253acef2aa050191a6755064`.

SHA256 padding/packing is implemented directly in `../crypto/src/sha.nr`, using Noir's constrained SHA256 compression black box. No manually constrained Brillig helper is used there. The initial upstream SHA package was removed after the pinned compiler reported constraint-coverage warnings.

Upstream `oracle_tests.nr` contains a test-only reference oracle declaration with an empty body; it is not imported or called by the complete hand relation. All hand Keccak calls execute the constrained branch of the official implementation. The upstream oracle test runner is not part of this project’s acceptance results.
