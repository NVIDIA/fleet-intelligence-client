# Contributing to Fleet Intelligence Client

If you are interested in contributing to Fleet Intelligence Client, your
contributions will usually fall into three categories:
1. You want to report a bug, feature request, or documentation issue
    - File an [issue](https://github.com/NVIDIA/fleet-intelligence-client/issues/new/choose)
    describing what you encountered or what you want to see changed.
    - When reporting a bug, please include your OS and architecture, the output
    of `nvfleetint version`, and how you installed the tool.
    - The maintainers will evaluate the issues and triage them, scheduling
    them for a release. If you believe the issue needs priority attention
    comment on the issue to notify the team.
2. You want to propose a new feature and implement it
    - Post about your intended feature, and maintainers will discuss the design and
    implementation.
    - Once the plan is agreed, implement it using the
    [code contributions](#code-contributions) guide below.
3. You want to implement a feature or bug-fix for an outstanding issue
    - Follow the [code contributions](#code-contributions) guide below.
    - If you need more context on a particular issue, ask in the issue.

## Code contributions

### Your first issue

1. Read the project's [README.md](https://github.com/NVIDIA/fleet-intelligence-client/blob/main/README.md)
    and [documentation](https://github.com/NVIDIA/fleet-intelligence-client/tree/main/docs)
    to understand the clients and their public behavior.
2. Find an issue to work on. The best way is to look for the [good first issue](https://github.com/NVIDIA/fleet-intelligence-client/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
    or [help wanted](https://github.com/NVIDIA/fleet-intelligence-client/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22) labels
3. Comment on the issue saying you are going to work on it.
4. Get familiar with the repository layout and the design notes in
    [docs/ARCHITECTURE.md](https://github.com/NVIDIA/fleet-intelligence-client/blob/main/docs/ARCHITECTURE.md).
5. Code! Make sure to update unit tests!
6. When done, [open your pull request](https://github.com/NVIDIA/fleet-intelligence-client/compare).
7. Verify that CI passes all [GitHub Actions checks](https://docs.github.com/en/actions), or fix if needed.
8. Wait for other developers to review your code and update code as needed.
9. Once reviewed and approved, a maintainer will merge your pull request.

Remember, if you are unsure about anything, don't hesitate to comment on issues and ask for clarifications!

### Development workflow

Install the Go version declared in `go.mod` or newer. Then run:

```bash
make build          # build bin/nvfleetint
make test           # run unit tests
make lint           # check formatting, vet, and lint
make test-coverage  # enforce the 80% coverage threshold
```

Run `make check` before opening a pull request. Use `make generate` after
changing the OpenAPI contract or generator configuration; do not edit generated
code directly.

### Managing PR labels

Each PR must be labeled according to whether it is a "breaking" or "non-breaking" change (using GitHub labels). This is used to highlight changes that users should know about when upgrading.

For nvfleetint, a "breaking" change is one that modifies the public Go API (the `nvfleetint`
package or the `nvfleetint` command surface) in a non-backward-compatible way. Internal packages
(`internal/`) carry no backward-compatibility expectation, so changes to them are not typically considered
breaking. Backward-compatible additions (such as a new optional flag or a new function) do not need to be
labeled.

Additional labels must be applied to indicate whether the change is a feature, improvement, bugfix, or documentation change.

### Signing Off Your Work

* We require that all contributors "sign-off" on their commits. This certifies that the contribution is your original work, or you have rights to submit it under the same license, or a compatible license.
  * Any contribution which contains commits that are not Signed-Off will not be accepted.
* To sign off on a commit you simply use the `--signoff` (or `-s`) option when committing your changes:
  ```bash
  $ git commit -s -m "Add cool feature."
  ```
  This will append the following to your commit message:
  ```
  Signed-off-by: Your Name <your@email.com>
  ```
* Full text of the DCO (https://developercertificate.org/):
  ```
    Developer Certificate of Origin
    Version 1.1

    Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

    Everyone is permitted to copy and distribute verbatim copies of this
    license document, but changing it is not allowed.


    Developer's Certificate of Origin 1.1

    By making a contribution to this project, I certify that:

    (a) The contribution was created in whole or in part by me and I
        have the right to submit it under the open source license
        indicated in the file; or

    (b) The contribution is based upon previous work that, to the best
        of my knowledge, is covered under an appropriate open source
        license and I have the right under that license to submit that
        work with modifications, whether created in whole or in part
        by me, under the same open source license (unless I am
        permitted to submit under a different license), as indicated
        in the file; or

    (c) The contribution was provided directly to me by some other
        person who certified (a), (b) or (c) and I have not modified
        it.

    (d) I understand and agree that this project and the contribution
        are public and that a record of the contribution (including all
        personal information I submit with it, including my sign-off) is
        maintained indefinitely and may be redistributed consistent with
        this project or the open source license(s) involved.
  ```

### Local Git hooks

Local hooks are optional but recommended. They check commit subject formatting
and scan staged files for secrets before commit.

Install `trufflehog` before enabling the hooks:

```bash
brew install trufflehog
make setup-git-hooks
```

You can run the hooks manually with:

```bash
make test-git-hooks
```

### Seasoned developers

Once you have gotten your feet wet and are more comfortable with the code, you
can look at the prioritized issues for the next release in the
[open issues](https://github.com/NVIDIA/fleet-intelligence-client/issues).

Look at the unassigned issues, and find an issue you are comfortable with
contributing to. Start with _Step 3_ from above, commenting on the issue to let
others know you are working on it. If you have any questions related to the
implementation of the issue, ask them in the issue instead of the PR.

### Branches and versions

The `main` branch is the active development branch and the target for pull
requests unless maintainers say otherwise. CI runs against pull requests to
`main` and must pass before merge.

Release branches may be created for stabilization or hotfix work when needed.
If a release branch exists, maintainers will document the target branch in the
issue or release notes.

### Branch naming

Branches used to create PRs should have a name of the form `<type>/<name>`,
where `<type>` is one of:

- `feat` for new features.
- `fix` for bugs or regressions.
- `docs` for documentation changes.
- `test` for test-only changes.
- `chore` for maintenance.

Use a short dash-separated name after the slash, for example `feat/node-tags`
or `docs/report-examples`.

### Release notes

Pull request titles are used to generate release notes. Make them concise and
descriptive of the user-visible change.
