<!--  Thanks for sending a pull request! -->

## Title
Controller-based repository sync: implementation, testing, and smart defaults

---

## Description
- **What changed:**  
  - **New controller-based sync architecture**: Dedicated repository controller with direct sync model
  - **Embedded controller support**: Runs in porch-server for CR cache deployments
  - **Smart defaults**: Auto-selects sync mode based on cache type (CR→controller, DB→legacy)
  - **API refactoring**: Moved Repository API to controllers/repositories
  - **Status improvements**: Added NextFullSyncTime field and Branch/Age printer columns
  - **Simplified condition messages**: Removed inline timestamps, using structured status fields
  - **Comprehensive testing**: Integration tests, refactored e2e tests
  - **Code cleanup**: Removed legacy event-based sync code
  - **RBAC hardening**: Events permissions reduced to read-only

- **Why it's needed:**  
  - Modernizes sync with Kubernetes-native controller pattern
  - Improves reliability and observability
  - Simplifies CR cache deployment
  - Maintains DB cache backward compatibility

- **How it works:**  
  - Controller watches Repository CRs and performs direct sync
  - CR cache: embedded controller (use-legacy-sync=false default)
  - DB cache: legacy sync (use-legacy-sync=true default)
  - Override with `--use-legacy-sync` flag

---

## Related Issue(s)
- Part of controller-based sync implementation and legacy sync deprecation effort

---

## Type of Change
- [x] New feature
- [x] Enhancement
- [x] Refactor
- [x] Tests
- [x] Documentation
- [ ] Bug fix
- [ ] Other: ________

---

## Checklist
- [x] Code follows project style guidelines  
- [x] Self-reviewed changes  
- [x] Tests added/updated  
- [x] Documentation added/updated  
- [x] All tests and gating checks pass  

---

## Testing Instructions (Optional)
1. **CR cache**: Deploy with `--cache-type=CR` (auto-enables controller sync)
2. **DB cache**: Deploy with `--cache-type=DB` (auto-enables legacy sync)
3. **Run tests**: `make test-integration` and e2e tests

---

## Additional Notes (Optional)
- **Further improvements:** Complete legacy sync deprecation, add controller metrics
- **Review notes:** Significant architectural change with 24 commits, maintains backward compatibility
