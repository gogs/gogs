# Flamego Migration Documentation

This directory contains comprehensive documentation for migrating Gogs from Macaron to Flamego.

## Quick Navigation

### 📋 Start Here
- **[FLAMEGO_MIGRATION_SUMMARY.md](./FLAMEGO_MIGRATION_SUMMARY.md)** - Read this first! Answers the core question and provides executive summary

### 📚 Detailed Guides
- **[macaron_to_flamego_migration.md](./macaron_to_flamego_migration.md)** - Complete migration guide with strategy, timeline, and solutions
- **[flamego_migration_examples.md](./flamego_migration_examples.md)** - Side-by-side code examples showing before/after patterns
- **[flamego_migration_checklist.md](./flamego_migration_checklist.md)** - Step-by-step execution checklist with daily tasks
- **[flamego_quick_reference.md](./flamego_quick_reference.md)** - Quick lookup tables for common patterns and APIs

## Document Purposes

| Document | Purpose | Best For |
|----------|---------|----------|
| **SUMMARY** | Decision making | Management, stakeholders |
| **Migration Guide** | Understanding approach | Tech leads, architects |
| **Code Examples** | Implementation reference | Developers during coding |
| **Checklist** | Execution tracking | Project managers, developers |
| **Quick Reference** | Quick lookups | All developers during migration |

## Reading Order

### For Decision Makers
1. Read: FLAMEGO_MIGRATION_SUMMARY.md
2. Scan: macaron_to_flamego_migration.md (focus on risks/benefits)
3. Review: flamego_migration_checklist.md (focus on timeline)

### For Project Managers
1. Read: FLAMEGO_MIGRATION_SUMMARY.md
2. Read: flamego_migration_checklist.md (execution plan)
3. Reference: macaron_to_flamego_migration.md (technical details)

### For Developers
1. Read: FLAMEGO_MIGRATION_SUMMARY.md (overview)
2. Study: flamego_migration_examples.md (learn patterns)
3. Reference: flamego_quick_reference.md (during coding)
4. Follow: flamego_migration_checklist.md (track progress)

### For Reviewers
1. Read: FLAMEGO_MIGRATION_SUMMARY.md
2. Reference: flamego_quick_reference.md
3. Check: flamego_migration_examples.md (verify patterns used)

## Key Questions Answered

### "Should we migrate?"
✅ Yes - see [FLAMEGO_MIGRATION_SUMMARY.md](./FLAMEGO_MIGRATION_SUMMARY.md)
- Complete feature parity
- Better performance
- Active development
- Official successor

### "What's involved?"
📋 See [macaron_to_flamego_migration.md](./macaron_to_flamego_migration.md)
- 8 phases over 20-25 days
- ~150-200 files to modify
- Comprehensive testing required

### "How do I do X in Flamego?"
🔍 See [flamego_quick_reference.md](./flamego_quick_reference.md)
- Quick lookup tables
- Common patterns
- Method mappings

### "What does the code look like?"
💻 See [flamego_migration_examples.md](./flamego_migration_examples.md)
- Side-by-side comparisons
- Complete working examples
- Real-world scenarios

### "What's the step-by-step process?"
✅ See [flamego_migration_checklist.md](./flamego_migration_checklist.md)
- Day-by-day tasks
- Testing procedures
- Rollback procedures

## Migration at a Glance

### Timeline
```
Phase 1: Dependencies        [1 day]    ████
Phase 2: Core Framework      [2-3 days] ████████
Phase 3: Context System      [2-3 days] ████████
Phase 4: Form Binding        [2 days]   ████
Phase 5: Route Handlers      [7 days]   ████████████████████
Phase 6: Testing             [4 days]   ████████████
Phase 7: Cleanup             [2 days]   ████
Phase 8: Deployment          [2 days]   ████
                             ─────────
                             Total: 20-25 days
```

### Feature Parity

| Feature | Macaron | Flamego | Status |
|---------|---------|---------|--------|
| Core framework | ✅ | ✅ | Full parity |
| Routing | ✅ | ✅ | Enhanced in Flamego |
| Middleware | ✅ | ✅ | All available |
| Session | ✅ | ✅ | Full parity |
| CSRF | ✅ | ✅ | Full parity |
| Cache | ✅ | ✅ | Full parity |
| i18n | ✅ | ✅ | Full parity |
| Forms | ✅ | ✅ | Full parity |
| Templates | ✅ | ✅ | Full parity |
| Toolbox | ✅ | ⚠️ | Easy to replace |

**Overall: ✅ 99% feature parity** (only toolbox needs custom code)

### Files to Modify

```
Core setup:              10 files
Route handlers:         100+ files
Forms:                   6 files
Tests:                  50+ files
Documentation:          10+ files
                        ─────────
Total:                  ~180-200 files
```

### Risk Assessment

| Risk Level | Description | Mitigation |
|------------|-------------|------------|
| 🟢 Low | Technical feasibility | Clear migration path documented |
| 🟡 Medium | Time commitment | 3-4 weeks allocated |
| 🟡 Medium | Testing burden | Comprehensive test plan included |
| 🟢 Low | Rollback difficulty | Easy git revert, backup plan ready |
| 🟢 Low | Missing features | All features available |

### Success Criteria

✅ All tests pass  
✅ Performance equal or better  
✅ No security regressions  
✅ No functionality lost  
✅ Zero critical bugs (first 2 weeks)

## External Resources

- [Flamego Official Docs](https://flamego.dev/)
- [Flamego GitHub](https://github.com/flamego/flamego)
- [Flamego Middleware](https://github.com/flamego)
- [Flamego Examples](https://github.com/flamego/flamego/tree/main/_examples)
- [Macaron to Flamego FAQ](https://flamego.dev/faqs.html#how-is-flamego-different-from-macaron)

## Quick Comparisons

### Import Changes
```go
// Before
import "gopkg.in/macaron.v1"

// After  
import "github.com/flamego/flamego"
```

### Route Syntax
```go
// Before
m.Get("/:username/:repo", handler)

// After
f.Get("/<username>/<repo>", handler)
```

### Handler Signature
```go
// Before
func Handler(c *macaron.Context) { }

// After
func Handler(c flamego.Context) { }
```

### Parameter Access
```go
// Before
username := c.Params(":username")

// After
username := c.Param("username")
```

## Support

### Questions?
- Read the documentation in order listed above
- Check the quick reference for specific patterns
- Review code examples for implementation details

### Found an Issue?
- Document in the checklist notes section
- Update examples if solution found
- Share with team

### Need Help?
- Flamego community: https://github.com/flamego/flamego/discussions
- Flamego issues: https://github.com/flamego/flamego/issues

## Document Metadata

| Document | Size | Last Updated | Status |
|----------|------|--------------|--------|
| FLAMEGO_MIGRATION_SUMMARY.md | 10 KB | 2026-01-25 | ✅ Complete |
| macaron_to_flamego_migration.md | 19 KB | 2026-01-25 | ✅ Complete |
| flamego_migration_examples.md | 27 KB | 2026-01-25 | ✅ Complete |
| flamego_migration_checklist.md | 17 KB | 2026-01-25 | ✅ Complete |
| flamego_quick_reference.md | 15 KB | 2026-01-25 | ✅ Complete |
| **Total** | **88 KB** | | **Ready for use** |

## License

These documents are part of the Gogs project and follow the same license.

## Contributing

If you find errors or have improvements:
1. Make corrections
2. Update relevant documents
3. Ensure consistency across all docs
4. Submit PR

---

**Ready to start?** → Begin with [FLAMEGO_MIGRATION_SUMMARY.md](./FLAMEGO_MIGRATION_SUMMARY.md)
