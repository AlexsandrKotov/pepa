# PEPA Governance

This document defines the governance model for the PEPA open-source project.

## Overview

PEPA is a community-driven open-source project focused on building a Platform Engineering & Pipeline Automator. The project follows a merit-based governance model that encourages contributions and rewards sustained, high-quality involvement.

## Roles

### Contributor

Anyone who contributes to the project — code, documentation, bug reports, feature requests, testing, or community support.

**Requirements**: None. Submit a PR, file an issue, or help in community channels.

**Privileges**:
- Submit pull requests and issues
- Participate in discussions and code reviews
- Vote in community decisions

### Reviewer

Contributors who have demonstrated understanding of the codebase and review quality.

**Requirements**:
- 3+ merged pull requests
- Demonstrated understanding of PEPA architecture
- Active in code review for at least 1 month

**Privileges**:
- Requested as reviewers on PRs
- Can approve PRs (counts toward merge requirements)
- Label and triage issues

### Approver

Experienced reviewers who can approve changes in their area of expertise.

**Requirements**:
- 10+ approved reviews
- Consistent high-quality contributions for 3+ months
- Nominated by a Committer and approved by the TOC

**Privileges**:
- Approve PRs in their area of expertise
- Merge PRs after approval window
- Propose RFCs for significant changes

### Committer

Trusted contributors with merge access to the main branch.

**Requirements**:
- Consistent, high-quality contributions for 6+ months
- Demonstrated architectural understanding
- Nominated by the TOC

**Privileges**:
- Merge pull requests to `main`
- Create and manage branches
- Tag releases
- Vote on TOC decisions

### Technical Oversight Committee (TOC)

The TOC provides technical leadership and final authority on project direction.

**Composition**: 5-7 members, elected annually from the Committer pool.

**Responsibilities**:
- Set technical direction and architecture decisions
- Manage CNCF relationship and submissions
- Approve new Committers and Reviewers
- Resolve disputes and enforce Code of Conduct
- Final authority on code merges to `main`

**Current TOC Members**:
- _To be announced at v0.1.0 launch_

**Elections**: Annual elections held in Q1. All Committers are eligible to vote.

## Special Interest Groups (SIGs)

SIGs are focused groups that own specific areas of the project:

| SIG | Scope | Focus Areas |
|-----|-------|-------------|
| **SIG: Plugin Ecosystem** | Plugin SDK, registry, certification | Plugin development experience, multi-language SDKs, OCI distribution |
| **SIG: Workflow Engine** | DAG execution, visual designer, templates | Workflow reliability, new step types, performance |
| **SIG: AI/ML** | RAG framework, AI tools, LLM integration | Tool accuracy, provider support, prompt engineering |
| **SIG: Security & RBAC** | Authentication, authorization, audit | Vault integration, compliance, penetration testing |
| **SIG: Infrastructure** | Helm chart, Docker, CI/CD | HA deployments, multi-cloud, observability |

**SIG Participation**: Open to all Contributors. SIGs meet bi-weekly and report to the TOC monthly.

## Decision Making

### Consensus Process

1. **Proposal**: Open an RFC issue or discuss in the relevant SIG
2. **Discussion**: Minimum 1-week discussion period for significant changes
3. **Consensus**: SIG members and Committers reach consensus
4. **Vote**: If consensus cannot be reached, TOC votes (simple majority)
5. **Decision**: TOC publishes the decision with rationale

### Types of Decisions

| Decision Type | Authority | Process |
|---------------|-----------|---------|
| Bug fix | Any Committer | Standard PR review |
| Feature (minor) | SIG + 1 Committer | PR review, SIG approval |
| Feature (major) | TOC | RFC, SIG discussion, TOC vote |
| Architecture change | TOC | RFC, 1-week discussion, TOC vote |
| Release | TOC + Committers | Release checklist, sign-off |
| Governance change | TOC | RFC, 2-week discussion, TOC supermajority |

## Contribution Process

```
Contributor -> PR -> CI Checks -> Reviewer -> Approver -> 24h Window -> Merge
```

1. **CI Checks**: All automated checks must pass (lint, test, security scan)
2. **Human Review**: At least 1 Reviewer approval required
3. **Approval**: Area Approver signs off
4. **Review Window**: 24-hour waiting period for additional feedback
5. **Merge**: Any Committer merges after the window

### Breaking Changes

Breaking changes require:
1. RFC document posted as a GitHub issue
2. Discussion in the relevant SIG
3. Migration guide or deprecation plan
4. TOC approval

## Release Process

| Release Type | Cadence | Authority |
|-------------|---------|-----------|
| Patch (x.y.Z) | As needed | Any Committer |
| Minor (x.Y.0) | Monthly | TOC sign-off |
| Major (X.0.0) | As needed | TOC vote + 2-week RFC |

All releases follow semantic versioning and include:
- Changelog with conventional commits
- Signed Git tags
- Signed plugin binaries
- Docker images pushed to GHCR
- Helm chart published to OCI registry

## Communication

| Channel | Purpose |
|---------|---------|
| GitHub Issues | Bug reports, feature requests |
| GitHub Discussions | Q&A, ideas, show & tell |
| Slack / Discord | Real-time chat, community help |
| SIG Meetings | Bi-weekly, open to all |
| Community Call | Monthly, roadmap + demos |
| Annual Summit | Governance elections, deep dives |

## Code of Conduct

All participants are bound by the [Contributor Covenant v2.1](CODE_OF_CONDUCT.md). Violations are handled by the TOC.

## License

PEPA is licensed under the [Apache License 2.0](LICENSE). All contributions are made under this license.

## Changes to This Document

Changes to this governance document require:
1. A pull request proposing the change
2. 2-week public comment period
3. Supermajority (2/3) TOC approval
4. Merge by TOC member
