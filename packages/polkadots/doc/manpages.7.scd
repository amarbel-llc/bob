manpages(7)

# NAME

manpages - conventions for section-7 manpages in the batman package

# DESCRIPTION

The batman package in bob builds section-7 manpages from *scdoc* source
files. Each manpage documents a BATS library or a design pattern. This
manpage describes the conventions for adding new manpages.

# SCDOC FORMAT

Manpages are written in scdoc format (*.7.scd* files) and compiled to troff
during the Nix build. Every manpage follows this section order:

. Header line: _name_(*7*)
. *NAME* — single-line summary: _name_ - _description_
. *DESCRIPTION* — overview of what the library or pattern covers
. *FUNCTIONS* or topic-specific sections — main documentation
. *EXAMPLE* — concrete code block demonstrating usage
. *SEE ALSO* — cross-references to related manpages

*Formatting conventions:*

- \*bold\* for command names, function names, and emphasis
- \_italic\_ for placeholders and parameters
- Triple backticks for code blocks
- Indented paragraphs under bold headings for definition lists

# LIBRARY-ATTACHED MANPAGES

BATS libraries store their manpage alongside their source code:

```
packages/batman/lib/bats-emo/
├── doc/
│   └── bats-emo.7.scd
├── load.bash
└── src/
    └── emo.bash
```

The manpage name matches the library directory name. The Nix derivation
builds both the library and the manpage together.

Two derivation styles exist for libraries:

*With automatic unpacking* (bats-support, bats-assert, tap-writer):

```
bats-support = pkgs.stdenvNoCC.mkDerivation {
  pname = "bats-support";
  version = "0.3.0";
  src = "${src}/lib/bats-support";
  nativeBuildInputs = [ pkgs.scdoc ];
  dontBuild = true;
  installPhase = ''
    mkdir -p $out/share/man/man7
    scdoc < doc/bats-support.7.scd > $out/share/man/man7/bats-support.7
  '';
};
```

File paths in the install phase are relative because the source is
automatically unpacked.

*With dontUnpack* (bats-island, bats-emo):

```
bats-emo = pkgs.stdenvNoCC.mkDerivation {
  pname = "bats-emo";
  version = "0.1.0";
  src = "${src}/lib/bats-emo";
  nativeBuildInputs = [ pkgs.scdoc ];
  dontUnpack = true;
  dontBuild = true;
  installPhase = ''
    mkdir -p $out/share/man/man7
    scdoc < $src/doc/bats-emo.7.scd > $out/share/man/man7/bats-emo.7
  '';
};
```

File paths use the *$src* prefix because no unpacking occurs.

Both styles install the compiled manpage to
*$out/share/man/man7/<name>.7*.

Library manpages are included in the *bats-libs* symlinkJoin
automatically because the library derivation itself produces the manpage.

# STANDALONE MANPAGES

Design-pattern manpages that are not attached to a BATS library live in
the *polkadots* package under *packages/polkadots/doc/*:

```
packages/polkadots/doc/
├── justfile-design-patterns.7.scd
└── manpages.7.scd
```

The polkadots Nix derivation compiles all *.7.scd* files in the *doc/*
directory using a loop:

```
polkadots = pkgs.stdenvNoCC.mkDerivation {
  pname = "polkadots";
  version = "0.1.0";
  src = ./packages/polkadots;
  nativeBuildInputs = [ pkgs.scdoc ];
  dontUnpack = true;
  dontBuild = true;
  installPhase = ''
    mkdir -p $out/share/man/man7
    for f in $src/doc/*.7.scd; do
      scdoc < "$f" > "$out/share/man/man7/$(basename "$f" .scd)"
    done
  '';
};
```

To add a new standalone manpage, create a *.7.scd* file in
*packages/polkadots/doc/* — no Nix changes required.

# ADDING A LIBRARY MANPAGE

. Create *packages/batman/lib/<name>/doc/<name>.7.scd*.
. Add a *scdoc* compile step to the library's derivation in
  *lib/packages/batman.nix*.
. The library is already in *bats-libs* — no further wiring needed.

# ADDING A STANDALONE MANPAGE

. Create *packages/polkadots/doc/<name>.7.scd*.
. Done. The polkadots derivation compiles all *.7.scd* files automatically.

# SEE ALSO

*justfile-design-patterns*(7), *bats-emo*(7), *bats-assert*(7)
