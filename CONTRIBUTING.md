# Contributing to Fleet Intelligence Client

If you are interested in contributing to Fleet Intelligence Client, your contributions will fall
into three categories:
1. You want to report a bug, feature request, or documentation issue
    - File an [issue](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/issues/new)
    describing what you encountered or what you want to see changed.
    - When reporting a bug, please include your OS and architecture, the output
    of `nvfleetctl version`, and how you installed the tool.
    - The maintainers will evaluate the issues and triage them, scheduling
    them for a release. If you believe the issue needs priority attention
    comment on the issue to notify the team.
2. You want to propose a new Feature and implement it
    - Post about your intended feature, and we shall discuss the design and
    implementation.
    - Once we agree that the plan looks good, go ahead and implement it, using
    the [code contributions](#code-contributions) guide below.
3. You want to implement a feature or bug-fix for an outstanding issue
    - Follow the [code contributions](#code-contributions) guide below.
    - If you need more context on a particular issue, please ask and we shall
    provide.

## Code contributions

### Your first issue

1. Read the project's [README.md](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/blob/main/README.md)
    to learn how to setup the development environment.
2. Find an issue to work on. The best way is to look for the [good first issue](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/issues?label_name[]=good%20first%20issue)
    or [help wanted](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/issues?label_name[]=help%20wanted) labels
3. Comment on the issue saying you are going to work on it.
4. Get familiar with the repository layout and the build/test workflow (`make build`, `make test`, `make lint`) described in the [README.md](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/blob/main/README.md), and the design notes in [docs/](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/tree/main/docs).
5. Code! Make sure to update unit tests!
6. When done, [create your merge request](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/merge_requests/new).
7. Verify that CI passes all [pipeline checks](https://docs.gitlab.com/ee/ci/pipelines/), or fix if needed.
8. Wait for other developers to review your code and update code as needed.
9. Once reviewed and approved, a maintainer will merge your merge request.

Remember, if you are unsure about anything, don't hesitate to comment on issues and ask for clarifications!

### Managing PR labels

Each MR must be labeled according to whether it is a "breaking" or "non-breaking" change (using GitLab labels). This is used to highlight changes that users should know about when upgrading.

For nvfleetctl, a "breaking" change is one that modifies the public Go API (the `pkg/fleetintelligence`
package or the `nvfleetctl` command surface) in a non-backward-compatible way. Internal packages
(`internal/`) carry no backward-compatibility expectation, so changes to them are not typically considered
breaking. Backward-compatible additions (such as a new optional flag or a new function) do not need to be
labeled.

Additional labels must be applied to indicate whether the change is a feature, improvement, bugfix, or documentation change.

### Seasoned developers

Once you have gotten your feet wet and are more comfortable with the code, you
can look at the prioritized issues for the next release in the
[open issues](https://gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/-/issues).

Look at the unassigned issues, and find an issue you are comfortable with
contributing to. Start with _Step 3_ from above, commenting on the issue to let
others know you are working on it. If you have any questions related to the
implementation of the issue, ask them in the issue instead of the PR.

### Branches and Versions

The nvfleetctl repository has two main branches:

1. `main` branch: it contains the last released version. Only hotfixes are targeted and merged into it.
2. `branch-x.y`: it is the development branch which contains the upcoming release. All the new features should be based on this branch and Merge/Pull request should target this branch (with the exception of hotfixes).

### Additional details

For every new version `x.y` of nvfleetctl there is a corresponding branch called `branch-x.y`, from where new feature development starts and PRs will be targeted and merged before its release. The exceptions to this are the 'hotfixes' that target the `main` branch, which target critical issues raised by GitLab users and are directly merged to `main` branch, and create a new subversion of the project. While trying to patch an issue which requires a 'hotfix', please state the intent in the MR.

For all development, your changes should be pushed into a branch (created using the naming instructions below) in your own fork of nvfleetctl and then create a merge request when the code is ready.

A few days before releasing version `x.y` the code of the current development branch (`branch-x.y`) will be frozen and a new branch, 'branch-x+1.y' will be created to continue development.

### Branch naming

Branches used to create PRs should have a name of the form `<type>-<name>`
which conforms to the following conventions:
- Type:
    - fea - For if the branch is for a new feature(s)
    - enh - For if the branch is an enhancement of an existing feature(s)
    - bug - For if the branch is for fixing a bug(s) or regression(s)
- Name:
    - A name to convey what is being worked on
    - Please use dashes or underscores between words as opposed to spaces.

## Attribution
Portions adopted from https://github.com/pytorch/pytorch/blob/master/CONTRIBUTING.md
