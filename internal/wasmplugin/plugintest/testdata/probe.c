// Runtime and network fixture. Each mode uses actual WASI or vv host imports;
// HTTP results, request echoes and sandbox checks are not precomputed by Go.
#include "common.h"

static void fetch_pages(const char *settings) {
    const char *raw = text(settings, "request");
    const char *count_text = text(settings, "count");
    int count = *count_text ? atoi(count_text) : 1;
    int codes[32], last = -1;
    require(count >= 0 && count <= 32);
    for (int i = 0; i < count; i++) {
        codes[i] = http_send(raw, strlen(raw));
        if (codes[i] >= 0) last = codes[i];
    }
    if (*text(settings, "fail")) {
        puts("{\"error\":{\"message\":\"gave up\"}}");
        return;
    }
    printf("{\"workInfo\":{\"tags\":[\"codes=");
    for (int i = 0; i < count; i++) printf("%s%d", i ? "," : "", codes[i]);
    putchar('"');
    if (last >= 0) {
        printf(",\"meta="); escaped(read_http(last, http_meta)); putchar('"');
    }
    putchar(']');
    if (last >= 0) { printf(",\"title\":"); quoted(read_http(last, http_body)); }
    puts("}}");
}

static void echo(const char *req) {
    if (!strcmp(text(req, "operation"), "search")) {
        const char *query = text(req, "query");
        printf("{\"candidates\":[{\"id\":\"q:"); escaped(query);
        printf("\",\"url\":\"https://example.com/?q="); escaped(query);
        printf("\",\"title\":"); quoted(query);
        printf(",\"detail\":\"contract %d\"}]}\n", atoi(field(req, "contract")));
    } else {
        printf("{\"workInfo\":{\"title\":"); quoted(text(req, "id"));
        printf(",\"tags\":"); settings_tags(field(req, "settings")); puts("}}");
    }
}

static void sandbox(const char *mode) {
    if (!strcmp(mode, "hostname")) {
        FILE *f = fopen("/etc/hostname", "r");
        // An accidentally exposed file must make the Go assertion fail.
        printf("{\"workInfo\":{\"tags\":[\"%s\"]}}\n", f ? "opened" : "refused");
        if (f) fclose(f);
    } else {
        __wasi_size_t env_count, env_size, arg_count, arg_size;
        require(__wasi_environ_sizes_get(&env_count, &env_size) == 0);
        require(__wasi_args_sizes_get(&arg_count, &arg_size) == 0);
        printf("{\"workInfo\":{\"tags\":[%s]}}\n", env_count || arg_count ? "\"leaked\"" : "");
    }
}

int main(void) {
    const char *req = request();
    const char *mode = text(req, !strcmp(text(req, "operation"), "search") ? "query" : "id");
    const char *settings = field(req, "settings");
    if (!strcmp(mode, "loop")) {
        fputs("looping\n", stderr);
        for (;;) __asm__ volatile("");
    } else if (!strcmp(mode, "sleep")) {
        sleep_forever();
    } else if (!strcmp(mode, "grow")) {
        while (__builtin_wasm_memory_grow(0, 16) != (size_t)-1) {}
        __builtin_trap();
    } else if (!strcmp(mode, "flood")) {
        repeat(1, 'x', 2 << 20);
    } else if (!strcmp(mode, "stderr")) {
        repeat(2, 'e', 100 << 10);
        puts("{\"workInfo\":{\"title\":\"stderr\"}}");
    } else if (!strcmp(mode, "hostname") || !strcmp(mode, "env")) {
        sandbox(mode);
    } else if (!strcmp(mode, "http")) {
        fetch_pages(settings);
    } else if (!strcmp(mode, "cover")) {
        printf("{\"workInfo\":{\"title\":\"cover\",\"cover\":"); quoted(text(settings, "cover")); puts("}}");
    } else if (!strcmp(mode, "fail")) {
        fputs("reporting an error\n", stderr);
        printf("{\"error\":{\"message\":\"");
        for (int i = 0; i < 400; i++) putchar('m');
        puts("\"}}");
    } else if (!strcmp(mode, "exit")) {
        fputs("exiting with 3\n", stderr);
        exit(3);
    } else {
        echo(req);
    }
    return 0;
}
