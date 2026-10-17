// Shared support for the small WASI test fixtures. Input is a Go-marshaled
// request object with string settings; this is not a production JSON parser.
// Regenerate the checked-in modules with ./build.sh (WASI SDK 25).
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <wasi/api.h>

__attribute__((import_module("vv"), import_name("http_send")))
int http_send(const char *, unsigned);
__attribute__((import_module("vv"), import_name("http_meta")))
int http_meta(int, char *, unsigned);
__attribute__((import_module("vv"), import_name("http_body")))
int http_body(int, char *, unsigned);

static void require(int ok) {
    if (!ok) exit(99);
}

static void whitespace(const char **p) {
    while (**p && strchr(" \t\r\n", **p)) ++*p;
}

static unsigned hex4(const char **p) {
    static const char digits[] = "0123456789abcdef";
    unsigned n = 0;
    for (int i = 0; i < 4; i++) {
        const char *digit = strchr(digits, *(*p)++);
        require(digit != NULL);
        n = n * 16 + (unsigned)(digit - digits);
    }
    return n;
}

static char *string_value(const char **p) {
    whitespace(p);
    require(*(*p)++ == '"');
    char *out = malloc(strlen(*p) + 1), *dst = out;
    require(out != NULL);
    while (**p != '"') {
        require(**p != 0);
        unsigned char c = *(*p)++;
        if (c != '\\') { *dst++ = c; continue; }
        c = *(*p)++;
        if (c == 'u') {
            unsigned n = hex4(p);
            if (n >= 0xd800 && n <= 0xdbff) {
                require((*p)[0] == '\\' && (*p)[1] == 'u');
                *p += 2;
                unsigned low = hex4(p);
                require(low >= 0xdc00 && low <= 0xdfff);
                n = 0x10000 + ((n - 0xd800) << 10) + low - 0xdc00;
            }
            if (n >= 0x10000) *dst++ = 0xf0 | (n >> 18);
            if (n >= 0x800) *dst++ = (n < 0x10000 ? 0xe0 : 0x80) | ((n >> 12) & 63);
            if (n >= 0x80) *dst++ = (n < 0x800 ? 0xc0 : 0x80) | ((n >> 6) & 63);
            *dst++ = n < 0x80 ? n : 0x80 | (n & 63);
        } else {
            static const char escapes[] = "\"\\/bfnrt";
            const char *esc = strchr(escapes, c);
            require(c != 0 && esc != NULL);
            *dst++ = "\"\\/\b\f\n\r\t"[esc - escapes];
        }
    }
    ++*p;
    *dst = 0;
    return out;
}

static void skip_value(const char **p) {
    whitespace(p);
    if (**p == '"') { free(string_value(p)); return; }
    if (**p == '{' || **p == '[') {
        char close = *(*p)++ == '{' ? '}' : ']';
        whitespace(p);
        while (**p != close) {
            require(**p != 0);
            if (**p == ',' || **p == ':') ++*p;
            else skip_value(p);
            whitespace(p);
        }
        ++*p;
        return;
    }
    while (**p && !strchr(",]} \t\r\n", **p)) ++*p;
}

static const char *field(const char *obj, const char *key) {
    whitespace(&obj);
    if (*obj++ != '{') return "null";
    whitespace(&obj);
    while (*obj != '}') {
        char *name = string_value(&obj);
        whitespace(&obj);
        require(*obj++ == ':');
        whitespace(&obj);
        int matches = strcmp(name, key) == 0;
        free(name);
        if (matches) return obj;
        skip_value(&obj);
        whitespace(&obj);
        if (*obj != ',') break;
        ++obj;
    }
    return "null";
}

static const char *text(const char *obj, const char *key) {
    const char *value = field(obj, key);
    return *value == '"' ? string_value(&value) : "";
}

static const char *request(void) {
    static char input[32768];
    size_t n = fread(input, 1, sizeof(input) - 1, stdin);
    require(n < sizeof(input) - 1 && !ferror(stdin));
    input[n] = 0;
    return input;
}

static void escaped(const char *s) {
    for (; *s; s++) {
        unsigned char c = *s;
        if (c == '"' || c == '\\') printf("\\%c", c);
        else if (c < 32) printf("\\u%04x", c);
        else putchar(c);
    }
}

static void quoted(const char *s) {
    putchar('"'); escaped(s); putchar('"');
}

static void settings_tags(const char *settings) {
    // encoding/json sorts map keys before handing the request to the guest.
    putchar('[');
    int count = 0;
    if (*settings == '{') {
        settings++;
        whitespace(&settings);
        while (*settings != '}') {
            char *key = string_value(&settings);
            whitespace(&settings);
            require(*settings++ == ':');
            char *value = string_value(&settings);
            if (count++) putchar(',');
            putchar('"'); escaped(key); putchar('='); escaped(value); putchar('"');
            free(key); free(value);
            whitespace(&settings);
            if (*settings != ',') break;
            settings++;
        }
    }
    putchar(']');
}

static char *read_http(int handle, int (*read)(int, char *, unsigned)) {
    int n = read(handle, NULL, 0);
    require(n >= 0);
    char *buf = malloc((unsigned)n + 1);
    require(buf != NULL && read(handle, buf, n) == n);
    buf[n] = 0;
    return buf;
}

static void repeat(int fd, char c, unsigned n) {
    char *buf = malloc(n);
    require(buf != NULL);
    memset(buf, c, n);
    (void)write(fd, buf, n);
    free(buf);
}
