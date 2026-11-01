# Contributing

modhak is in the planning stage; there is no code yet. Useful contributions right now:

- **Run the verification checklist** at the end of [docs/FEATURES.md](docs/FEATURES.md) against a real MariaDB or MySQL instance and report results (version, flavor, query, output).
- Review the ADRs in [docs/decisions/](docs/decisions/) and the open questions at the end of [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

Ground rules
- Read [AGENTS.md](AGENTS.md) for conventions; they apply to people too.
- Claims about catalogs, variables, statements, and versions need a link to official MariaDB or MySQL documentation, or must be marked UNVERIFIED.
- Never add code that executes DDL against a target.
- Changes that contradict an ADR need a new ADR.
- Tests are table-driven; code that touches a catalog must work on, or explicitly degrade for, both MariaDB and MySQL.
- Open an issue before starting large work. Keep pull requests focused.

By contributing you agree your work is licensed under the project license (see [LICENSE](LICENSE)).
