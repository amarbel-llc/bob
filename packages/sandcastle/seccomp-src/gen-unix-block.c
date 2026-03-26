// Generates a cBPF filter that blocks socket(AF_UNIX, ...) syscalls.
// Writes raw BPF bytecode to stdout via seccomp_export_bpf().

#include <seccomp.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/socket.h>

int main(void) {
  scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
  if (!ctx) {
    fprintf(stderr, "seccomp_init failed\n");
    return 1;
  }

  // Block socket(AF_UNIX, ...) — arg0 == AF_UNIX
  if (seccomp_rule_add(ctx, SCMP_ACT_ERRNO(1), SCMP_SYS(socket), 1,
                       SCMP_A0(SCMP_CMP_EQ, AF_UNIX)) < 0) {
    fprintf(stderr, "seccomp_rule_add(socket) failed\n");
    seccomp_release(ctx);
    return 1;
  }

  // Block socketpair(AF_UNIX, ...) — arg0 == AF_UNIX
  if (seccomp_rule_add(ctx, SCMP_ACT_ERRNO(1), SCMP_SYS(socketpair), 1,
                       SCMP_A0(SCMP_CMP_EQ, AF_UNIX)) < 0) {
    fprintf(stderr, "seccomp_rule_add(socketpair) failed\n");
    seccomp_release(ctx);
    return 1;
  }

  if (seccomp_export_bpf(ctx, 1) < 0) { // fd 1 = stdout
    fprintf(stderr, "seccomp_export_bpf failed\n");
    seccomp_release(ctx);
    return 1;
  }

  seccomp_release(ctx);
  return 0;
}
