# Security Policy

## Supported versions

Tacita has no published releases. No version is currently supported for
production use.

Once releases begin, this table will identify supported versions and security
fix policy.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability.

After the repository is published, use a
[private GitHub security advisory](https://github.com/tuxerrante/tacita/security/advisories/new).
Until then, report the issue directly to the repository owner through an
already established private channel.

Include:

1. affected revision or version;
2. reproduction steps;
3. expected and observed behavior;
4. potential impact;
5. any suggested mitigation.

## Security boundary

Tacita analyzes repositories as untrusted input. The project aims to limit:

- Git option and command injection;
- parser confusion from unusual paths and metadata;
- resource exhaustion from large histories or pair expansion;
- unexpected behavior from local Git configuration;
- incomplete history presented as complete evidence;
- nondeterminism that changes findings across identical runs.

Tacita is a local process running with the invoking user's privileges. It is not
a sandbox, and local hooks are not a security boundary.
