# Changelog

## [0.1.1](https://github.com/Lemon4ksan/seal/compare/v0.1.0...v0.1.1) (2026-10-16)


### Features

* absorb VFS defenses, envelope encryption, and shamir secret sharing from foundation ([2008d23](https://github.com/Lemon4ksan/seal/commit/2008d23dd976b3c530d4aa41011e4e23aff10b16))
* add native c przm decoder ([1b9883e](https://github.com/Lemon4ksan/seal/commit/1b9883e3963bdf2e29df21107c8821c922a74105))
* add prism compression method ([e07c6bf](https://github.com/Lemon4ksan/seal/commit/e07c6bf0b591ace335a36407bc12da75d6a6e040))
* add stealth extension SDK, resumable checkpoints, UnRAR support & ([d2350d3](https://github.com/Lemon4ksan/seal/commit/d2350d395b6ad7bfce7eecf9a5a0136bffcb3206))
* add wim ([8a391a1](https://github.com/Lemon4ksan/seal/commit/8a391a1b15313d48cb10b84f1c73aaad829eec2c))
* **api:** make archiver polymorphic with string paths, auto-detect container, add Compress ([6e86640](https://github.com/Lemon4ksan/seal/commit/6e8664031856a0d56ac3fcd5485ceead7ff04356))
* **bench:** add brotli_battle arena benchmark suite and RFC 7932 dict registration ([3e9415f](https://github.com/Lemon4ksan/seal/commit/3e9415f48376a8417ed18deb854076a3af407a72))
* **bench:** update brotli_battle to benchmark 6 real production PC files ([2535f28](https://github.com/Lemon4ksan/seal/commit/2535f288df74541f7bb1aae0d8444e49ee07483f))
* **codec:** implement IBM z/OS CMPSC compression and expansion (Method 16) ([a4af57e](https://github.com/Lemon4ksan/seal/commit/a4af57e43f9cdbfdf9d476f14024f5721a7d390b))
* **codec:** implement legacy PKWARE decompression codecs in codec/legacy ([33d1f7d](https://github.com/Lemon4ksan/seal/commit/33d1f7dba8b2ffb3419afd3b5e2a481143107ae6))
* **codec:** implement PKWARE DCL Implode (Method 10 / blast) and 35-year benchmark suite ([1d6f389](https://github.com/Lemon4ksan/seal/commit/1d6f3898f5f5c5c96c29e7e5daba16cbbc60e0c6))
* **codec:** implement XZ stream container (Method 95 / LZMA2 + BCJ) ([7d881f2](https://github.com/Lemon4ksan/seal/commit/7d881f2e68259fab5eae5eb0dbd7f1b794c9f2fc))
* **codec:** integrate PRZM compressor with GPU acceleration support ([3910ad2](https://github.com/Lemon4ksan/seal/commit/3910ad287f2ffb165fcd98475aa293a8f6047cf6))
* **container:** add ISO 9660 & Joliet (.iso) disc image support ([66abb39](https://github.com/Lemon4ksan/seal/commit/66abb390f6d01bd5858a492b434a62cfb652daa1))
* **container:** add Microsoft Cabinet (.cab) format support ([2ff101d](https://github.com/Lemon4ksan/seal/commit/2ff101d4a19f417ad3a89326c703f8b7b3ad3444))
* **container:** add Unix AR and Debian package (.ar, .deb, .a) support ([c1c6254](https://github.com/Lemon4ksan/seal/commit/c1c6254e6420a52f69768134a88795274c73e39e))
* **container:** add Unix CPIO (.cpio) archive support ([316092f](https://github.com/Lemon4ksan/seal/commit/316092fb7719a589cdf2e2b39d143924b8e03f42))
* **core:** standardize on .eon v0.1, harden codecs & containers, ([a30c46c](https://github.com/Lemon4ksan/seal/commit/a30c46c275f4e220648c48d01d9e61a9021426f2))
* **encoding:** integrate text/encoding into zip, archiver and CLI with OEM fallback ([d3237e6](https://github.com/Lemon4ksan/seal/commit/d3237e6e7f4472a7c4f8f8839052ad9996b0c404))
* **eon:** implement EON 3.4 SMX self-mounting executable runtime and solid archiving ([a980c6d](https://github.com/Lemon4ksan/seal/commit/a980c6d7576f4271090815b01eb7f6ade2d977de))
* **filter:** port PNG transcoding from PRZM to Seal filter DAG ([f077522](https://github.com/Lemon4ksan/seal/commit/f077522280e35347dd4b8178cda93aa60f0cc38b))
* implement sevenzip ([b20fe44](https://github.com/Lemon4ksan/seal/commit/b20fe44a1cc9b3b0d3cb82ddb2961bde1bcc9408))
* integrate PRZM CUDA GPU acceleration via -gpu flag ([e6c6e52](https://github.com/Lemon4ksan/seal/commit/e6c6e52dfa1ab477289998f8e8892d20d720ba73))
* **seal:** integrate przm v0.3 codec, LRU chunk cache, and bytecode filter options ([69e00fb](https://github.com/Lemon4ksan/seal/commit/69e00fb352f84074f7cd70f9962cd00ad3bf89bd))
* **sevenzip:** port core 7-Zip features (AES-256 encryption, EncodedHeader, Inspector & Doctor) ([8653c05](https://github.com/Lemon4ksan/seal/commit/8653c05e8b841d4a73856405e56a8742da10621a))
* update cli ([8c55039](https://github.com/Lemon4ksan/seal/commit/8c5503953773674851e827660826b7b6d0eca2e4))
* **zip:** add automatic SFX (Self-Extracting Archive) stub offset detection ([631013f](https://github.com/Lemon4ksan/seal/commit/631013f8d9b9f2620ab9f74ba27b2b55349d7311))
* **zip:** enforce strict PKWARE APPNOTE v6.3.10 compliance and section annotations ([9b7c53f](https://github.com/Lemon4ksan/seal/commit/9b7c53fe55d86278530bb403e93bca569ce5a657))
* **zip:** implement Archive Inspector and Archive Doctor subsystems ([f451da3](https://github.com/Lemon4ksan/seal/commit/f451da3f134316f1f84bd879812c247fa0d958fe))
* **zip:** implement PKWARE APPNOTE v6.3.10 spec, iterators, and ([6fca4a0](https://github.com/Lemon4ksan/seal/commit/6fca4a02f7adeb780175c523eb916042698c34a2))


### Bug Fixes

* **archiver:** fix generic.Coalesce typo ([c96b086](https://github.com/Lemon4ksan/seal/commit/c96b0865568f879f05e0196c24093e7421b2bca9))
* **ci:** resolve all linter issues, add PR triggers, and align release-please to v0.1.0 ([e105931](https://github.com/Lemon4ksan/seal/commit/e1059314b0eb396eb6c704a10c5ec75f6d77d02f))
* **codec:** modernize przm integration with zero-alloc DecompressTo, Decode, and options API ([73890a4](https://github.com/Lemon4ksan/seal/commit/73890a46ed463d2c0742cfdce8b5f731358d218d))
* **eon:** avoid copying sync.Mutex in Reader SnapshotReader ([df4c431](https://github.com/Lemon4ksan/seal/commit/df4c4314ce7ebc0361db91027ad020038da0fc23))
* **eon:** enable GPU acceleration for PRZM and limit worker contention to prevent system lag ([f54b80a](https://github.com/Lemon4ksan/seal/commit/f54b80acb54e274fbba7136395552fbc20186023))
* **eon:** preserve directory locality for solid blocks unless solidExtension is set, and set GPU workers to 4 ([f035631](https://github.com/Lemon4ksan/seal/commit/f0356311fe3b93540e839ab8083a33413ee3e612))
* **przm:** align przm codec calls with modern przm API and decouple PNG filter ([9db8ca9](https://github.com/Lemon4ksan/seal/commit/9db8ca96467bb7ec8e9c3e908a1de9083259bb03))
* resolve module namespaces and test configuration ([4623668](https://github.com/Lemon4ksan/seal/commit/46236680edf6f54d733b57e891218b1d37178919))
* **sys:** remove unused unixNanoToWinFiletime helper on non-windows ([c4a0b45](https://github.com/Lemon4ksan/seal/commit/c4a0b450c70f54087bedd3b1db25e89717592073))
* update ast and bin imports to new foundation namespaces ([6c1ca30](https://github.com/Lemon4ksan/seal/commit/6c1ca30ae3c996684c228f38aff950bce39f0325))
* **zip:** allow empty archives by default with RFC 2119 / APPNOTE 4.1.2 annotation ([52f84a3](https://github.com/Lemon4ksan/seal/commit/52f84a3303309c0d50b40a3018490565f23b4c49))
