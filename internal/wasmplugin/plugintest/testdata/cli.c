// CLI fixture: read the real request, forward fetches through vv, and return
// the query, HTTP body and effective settings so the CLI tests can check them.
#include "common.h"

int main(void) {
    const char *req = request();
    if (!strcmp(text(req, "operation"), "search")) {
        const char *query = text(req, "query");
        if (!strcmp(query, "fail")) {
            fputs("no newline", stderr);
            puts("{\"error\":{\"message\":\"failed on purpose\"}}");
            return 0;
        }
        // source and name make the candidate a person plugin's too; a work-info plugin ignores them.
        printf("{\"candidates\":[{\"id\":\"site.test/works/1\",\"url\":\"https://site.test/works/1\",\"source\":\"s\",\"title\":");
        quoted(query); printf(",\"name\":"); quoted(query); puts("}]}");
        return 0;
    }
    const char *link = text(req, "url");
    if (*link) {
        // A person plugin's fetch by link names the person by the link.
        printf("{\"person\":{\"name\":"); quoted(link);
        puts(",\"externalIds\":[{\"source\":\"other\",\"value\":\"x\"}]}}");
        return 0;
    }
    const char *id = text(req, "id");
    const char *source = text(req, "source");
    if (*source) {
        // Only a person plugin's fetch carries a source.
        printf("{\"person\":{\"name\":"); quoted(id);
        printf(",\"externalIds\":[{\"source\":\"other\",\"value\":\"x\"},{\"source\":");
        quoted(source); printf(",\"value\":"); quoted(id); puts("}]}}");
        return 0;
    }
    char url[1024], send[2048];
    int n = snprintf(url, sizeof(url), "http://%s", id);
    require(n >= 0 && (unsigned)n < sizeof(url));
    fprintf(stderr, "fetching %s\n", url);
    // Test URLs contain no quotes or control characters.
    n = snprintf(send, sizeof(send), "{\"method\":\"GET\",\"url\":\"%s\"}", url);
    require(n >= 0 && (unsigned)n < sizeof(send));
    int handle = http_send(send, strlen(send));
    if (handle < 0) {
        printf("{\"error\":{\"message\":\"http_send returned %d\"}}\n", handle);
        return 0;
    }
    printf("{\"workInfo\":{\"title\":"); quoted(read_http(handle, http_body));
    printf(",\"tags\":"); settings_tags(field(req, "settings")); puts("}}");
    return 0;
}
