# Dead Ends Log

| Iteration | Approach Tried | Why It Failed | Files Touched |
|-----------|---------------|---------------|---------------|
| M1 Iter 1 | Disconnected Tropical structs & scan functions bypassing `encodeTrellisStream`; passing `nil` frequency to `KBConfluentSubstituteL1..L5` | Forensic Auditor reported INTEGRITY VIOLATION / CHEATING DETECTED. BlellochPrefixScan was a serial loop and critical pair resolution was dead code in production. | `sheaf_trellis.go`, `knuth_bendix.go`, `ggtc.go`, `pws.go` |
