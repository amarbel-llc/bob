// Reads a pre-generated cBPF filter from a file, applies it via
// prctl(PR_SET_SECCOMP), and execs the remaining argv.
//
// Usage: apply-seccomp <bpf-file> <command> [args...]
//
// Supports chaining multiple BPF files before the command:
//   apply-seccomp <bpf1> <bpf2> ... -- <command> [args...]
// Each file is applied as a separate seccomp filter. The kernel evaluates
// all filters and uses the most restrictive result.

#include <errno.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <unistd.h>

static int apply_bpf_file(const char *path) {
  FILE *f = fopen(path, "rb");
  if (!f) {
    fprintf(stderr, "apply-seccomp: cannot open %s: %s\n", path,
            strerror(errno));
    return -1;
  }

  fseek(f, 0, SEEK_END);
  long size = ftell(f);
  fseek(f, 0, SEEK_SET);

  if (size <= 0 || size % sizeof(struct sock_filter) != 0) {
    fprintf(stderr, "apply-seccomp: invalid BPF file size: %ld\n", size);
    fclose(f);
    return -1;
  }

  struct sock_filter *insns = malloc(size);
  if (!insns) {
    fprintf(stderr, "apply-seccomp: malloc failed\n");
    fclose(f);
    return -1;
  }

  if (fread(insns, 1, size, f) != (size_t)size) {
    fprintf(stderr, "apply-seccomp: short read on %s\n", path);
    free(insns);
    fclose(f);
    return -1;
  }
  fclose(f);

  struct sock_fprog prog = {
      .len = (unsigned short)(size / sizeof(struct sock_filter)),
      .filter = insns,
  };

  if (prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &prog) < 0) {
    fprintf(stderr, "apply-seccomp: PR_SET_SECCOMP failed: %s\n",
            strerror(errno));
    free(insns);
    return -1;
  }

  free(insns);
  return 0;
}

int main(int argc, char *argv[]) {
  if (argc < 3) {
    fprintf(stderr,
            "usage: apply-seccomp <bpf-file> [<bpf-file>...] [--] "
            "<command> [args...]\n");
    return 1;
  }

  // PR_SET_NO_NEW_PRIVS is required before PR_SET_SECCOMP
  if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) < 0) {
    fprintf(stderr, "apply-seccomp: PR_SET_NO_NEW_PRIVS failed: %s\n",
            strerror(errno));
    return 1;
  }

  // Apply BPF files until we hit "--" or a non-.bpf argument
  int i;
  for (i = 1; i < argc; i++) {
    if (strcmp(argv[i], "--") == 0) {
      i++;
      break;
    }

    // Check if this looks like a BPF file (ends with .bpf)
    size_t len = strlen(argv[i]);
    if (len < 5 || strcmp(argv[i] + len - 4, ".bpf") != 0) {
      // Not a .bpf file — treat as the start of the command
      break;
    }

    if (apply_bpf_file(argv[i]) < 0) {
      return 1;
    }
  }

  if (i >= argc) {
    fprintf(stderr, "apply-seccomp: no command specified\n");
    return 1;
  }

  execvp(argv[i], &argv[i]);
  fprintf(stderr, "apply-seccomp: exec failed: %s\n", strerror(errno));
  return 1;
}
