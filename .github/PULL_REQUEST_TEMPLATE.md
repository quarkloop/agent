## Description

<!-- What does this PR do and why? Link related issues with "Closes #N". -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor
- [ ] Documentation
- [ ] Chore / dependency update

## Checklist

- [ ] `make vet` passes
- [ ] `make test` passes
- [ ] `make arch-check` and `make dead-code-check` pass when architecture or package ownership changes
- [ ] New API/DTO types stay at the ingress boundary and are mapped into domain/runtime types before execution logic
- [ ] Slices, maps, raw JSON, and byte buffers crossing package boundaries are copied instead of reusing mutable backing storage
- [ ] Services do not call each other (agent coordinates multi-step flows)
- [ ] `docs/` changes are not staged (local task tracking only)
- [ ] Relevant documentation updated (README, AGENTS.md, code comments)
