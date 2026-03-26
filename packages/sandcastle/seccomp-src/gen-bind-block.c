// Generates a cBPF filter that blocks the bind() syscall.
// Writes raw BPF bytecode to stdout via seccomp_export_bpf().
//
// A blanket bind() block (not selective by AF family) is sufficient because:
// - socat binds its proxy ports BEFORE seccomp is applied via apply-seccomp
// - The filter only affects the user's command (MCP server)
// - seccomp-BPF cannot dereference pointers (sockaddr is a pointer arg)

#include <seccomp.h>
#include <stdio.h>
#include <stdlib.h>

int main(void) {
  scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
  if (!ctx) {
    fprintf(stderr, "seccomp_init failed\n");
    return 1;
  }

  if (seccomp_rule_add(ctx, SCMP_ACT_ERRNO(1), SCMP_SYS(bind), 0) < 0) {
    fprintf(stderr, "seccomp_rule_add(bind) failed\n");
    seccomp_release(ctx);
    return 1;
  }

  if (seccomp_export_bpf(ctx, 1) < 0) {
    fprintf(stderr, "seccomp_export_bpf failed\n");
    seccomp_release(ctx);
    return 1;
  }

  seccomp_release(ctx);
  return 0;
}
