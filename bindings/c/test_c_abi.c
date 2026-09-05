/*
 * Copyright (c) 2026 Lemon4ksan All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

#include <stdio.h>
#include "../../include/eon.h"

int main(int argc, char* argv[]) {
    printf("=== EON C ABI Verification ===\n");
    printf("libeon version: %s\n", eon_version());

    if (argc < 2) {
        printf("Usage: test_c_abi <archive.eon> [password]\n");
        return 0;
    }

    const char* path = argv[1];
    const char* password = (argc >= 3) ? argv[2] : NULL;

    int err = 0;
    eon_handle_t handle = eon_open(path, password, &err);
    if (handle == 0) {
        fprintf(stderr, "Failed to open '%s': %s (err code %d)\n", path, eon_get_last_error(), err);
        return 1;
    }

    int64_t count = eon_file_count(handle);
    printf("Successfully opened '%s'. Total extents: %lld\n", path, (long long)count);

    for (int64_t i = 0; i < count; i++) {
        eon_file_stat_t st;
        if (eon_stat_index(handle, i, &st) == 0) {
            printf("  [%lld] %s (%llu bytes, %s)\n",
                   (long long)i, st.name, (unsigned long long)st.size, st.is_dir ? "DIR" : "FILE");

            if (!st.is_dir && st.size > 0 && st.size < 1024) {
                char buf[1024];
                int64_t n = eon_read_file(handle, st.name, buf, sizeof(buf));
                if (n > 0) {
                    printf("        -> Read %lld bytes OK\n", (long long)n);
                }
            }
        }
    }

    eon_close(handle);
    printf("Container closed. Test passed!\n");
    return 0;
}
