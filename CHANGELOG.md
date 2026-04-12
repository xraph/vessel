# Changelog

All notable changes to this project will be documented in this file.

## [v1.0.1] - 2026-04-12

### Initial Release

- f59ef0f (HEAD -> main, origin/main) chore(deps): update go version to 1.25.0 and upgrade go-utils to v1.1.1
- 0a326d2 refactor: remove unused names function from typeRegistry
- 643761e feat: add settings.local.json for permission configurations
- 44e3f00 feat: add ProvideValue function for singleton service registration and deprecate old methods
- e8e5544 chore(deps): remove unused uber dependencies from go.mod and go.sum
- 712063a lint: formatted code
- cd0f8a1 refactor: breaking change to make the API stable
- 916a5bf feat: introduce eager instantiation support in constructor registration
- 2122099 feat: add support for service aliases in constructor registration
- db4a6f0 chore(deps): update go-utils dependency to v0.0.5 in go.sum
- 17e4617 chore(deps): update go-utils dependency to v0.0.5
- aa41455 feat: implement constructor injection framework with In/Out structs
- 180f346 docs: update benchmark results and performance characteristics in README
- ebb498d feat: add middleware support for service resolution and lifecycle management
- 9e2a6dd chore(deps): remove local replacement for go-utils and update go.sum
- 8fc60da refactor(error): improve type mismatch error messaging in Resolve functions
- 7950df4 refactor(di): streamline container creation and improve error messaging
- 36e2b83 feat(di): Implement Forge Dependency Injection system with service lifecycle management

