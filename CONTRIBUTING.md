# Contributing

modhak is in early development (see the roadmap in [docs/ROADMAP.md](docs/ROADMAP.md) for what exists). Useful contributions right now:

- **Run the verification checklist** at the end of [docs/FEATURES.md](docs/FEATURES.md) against a real MariaDB or MySQL instance and report results (version, flavor, query, output).
- Review the ADRs in [docs/decisions/](docs/decisions/) and the open questions at the end of [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

Ground rules
- Read [AGENTS.md](AGENTS.md) for conventions; they apply to people too.
- Claims about catalogs, variables, statements, and versions need a link to official MariaDB or MySQL documentation, or must be marked UNVERIFIED.
- Never add code that executes DDL against a target.
- Changes that contradict an ADR need a new ADR.
- Tests are table-driven; code that touches a catalog must work on, or explicitly degrade for, both MariaDB and MySQL.
- Open an issue before starting large work. Keep pull requests focused.

Cutting a release (maintainers)
- Releases are built from a tag. `git tag v0.1.0 && git push origin v0.1.0` runs [.github/workflows/release.yml](.github/workflows/release.yml): it runs the tests, builds static binaries for Linux, macOS and Windows with `scripts/release-build.sh`, and publishes a GitHub release with `SHA256SUMS` and generated notes. A tag with a suffix (`v0.1.0-rc1`) is a pre-release.
- To check the build without publishing, run the workflow manually from the Actions tab (it uploads the archives to the run), or run `make dist VERSION=v0.0.0-test` locally.
- Binaries are not signed or notarized.

By contributing you agree your work is licensed under the project license (see [LICENSE](LICENSE)).
