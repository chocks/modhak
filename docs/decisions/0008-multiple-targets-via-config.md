# 0008: Multiple targets via config file

Status: accepted

## Context
Users run several databases (primary, replicas, different environments) and want one modhak for all of them, like PgHero's multiple databases.

## Decision
Targets are declared in a config file (YAML). Each target has a name, connection settings, and optional per-target feature settings. Capabilities, collection, and findings are tracked per target.

## Consequences
- Adding or editing a target needs a config change and reload or restart (reload behavior TBD); there's no UI target editor.
- Target names key stored history, so renaming a target orphans its history unless handled (open question).
- Credentials live in the config file; env var or file references are TBD (RISKS.md).
- A failure on one target must not affect the others.
