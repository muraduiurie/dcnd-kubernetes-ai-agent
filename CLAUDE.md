# CLAUDE.md

## Working agreement (read first; overrides everything below)

Iurie Muradu is the author of this project. Claude is an architectural advisor,
a reviewer, and a typing machine. Claude is not a co-author and never decides
what gets built.

- **Do exactly what is ordered, nothing more.** "Create the API types" means the
  Go type definitions only. Not the manager, not the reconcilers, not the
  Makefile, not tests, not docs, not generated code. When the boundary of an
  order is unclear, take the narrower reading and say what was left out.
- **Never start unordered work.** No "while I'm here" edits, no side-effect
  files, no scaffolding, no `go mod tidy`, no code generation, no cluster
  commands, unless the order includes them.
- **Additions are proposals, never actions.** Put suggestions under a
  `Proposals` heading at the end of the reply, one line each. Do not implement
  a proposal until Iurie orders it.
- **Speak up on design.** If an order looks like a bad architectural decision,
  say so before writing anything: the reason and the alternative, briefly.
  Then stop and wait. Iurie is the final voice. Once he confirms, write it as
  ordered without further argument.
- **Handling an order:** (1) restate the scope in one line, (2) raise concerns
  if any and stop, (3) otherwise write exactly that scope, (4) report what was
  written, then stop.
- **Reviews review.** When asked to review, report findings. Do not fix them.
- **No git side effects.** No commits, branches, pushes, or tags unless ordered.

## The project

A 90-minute talk at Dutch Cloud Native Day, Jaarbeurs Utrecht, Friday
30 October 2026: *Building Production-Grade Kubernetes AI Agents*. The talk
shows a Kubernetes operator that runs AI agents as custom resources, built from
first principles. The full design synthesis, diagrams, and decision log live in
the blueprint artifact:
https://claude.ai/code/artifact/bd38a4bd-325f-4769-a337-2ef84b008f4e

Thesis in three lines. Detection is deterministic and free. Investigation is
probabilistic and paid. Application is deterministic and gated.

## Vocabulary (use these words in code, comments, and prose)

- **Agent definition** = the `AIAgent` resource. **Runtime** = the pod that
  executes runs. Never one word for both.
- **Tools** = read-side functions the runtime calls in-process with a borrowed
  token. **Actions** = the closed write-side vocabulary the operator executes.
  Never blur the two.
- **Proposal tools** = write-shaped tools the model calls (`set_image`,
  `rollback_deployment`) that validate arguments and append to
  `proposedActions`. They never apply anything.
- **Operator** = the controller binary with no model in it. It is the only
  component that writes into workload namespaces.

## Settled architecture

### Resources, group `ai.dncp.io/v1alpha1`

- `AIAgent`, cluster-scoped. Spec: `model` (provider, model, secretRef),
  `instructions`, `tools`, `permissions` (namespaces, resources; resources
  imply get/list/watch, no verbs field), `policy.approvalRequiredFor`,
  `limits` (maxSteps, maxTokensPerRun, timeout), `triggers` (Alertmanager
  match, objective template, cooldown, remediation mode),
  `maxConcurrentRuns`. Status: Ready condition and counters.
- `AgentRun`, namespaced, created in the workload namespace. Spec: `agentRef`,
  `objective`, `remediation.mode` (`propose` | `auto`), `approvals`
  (actionId + digest, written by humans), `decision` (declined + reason),
  `approvalTimeout`. Status: `phase`, `conditions`, `evidence[]`,
  `proposedActions[]` (id, type, target, parameters, evidenceRefs, digest,
  dryRun), `usage`, `attempts`, `executor`, `createdBy`.
- Phases: Pending, Accepted, Investigating, AwaitingApproval, Applying,
  Verifying, Succeeded. Exits: Rejected (admission), Failed (limits,
  ungrounded proposal, max attempts, verification timeout), Declined (human),
  Superseded (alert resolved before anything was applied).
- Keep `phase` for printer columns and `conditions` alongside it. Runtime and
  operator write status through server-side apply with distinct field managers.
- Runs are owned by their `AIAgent` and carry a `ttlSecondsAfterFinished`.

### Trust boundary and identities

- **Operator** (namespace `ai-system`): static RBAC shipped with its install.
  CRDs, provisioning of Deployments, ServiceAccounts, Roles, RoleBindings, the
  read set (escalation prevention requires it), patch on deployments,
  services, configmaps, delete on pods, plus the Alertmanager receiver. Three
  reconcilers in one binary: AIAgent, AgentRun, alert receiver.
- **Runtime** (one Deployment per AIAgent, in `ai-system`): its ServiceAccount
  holds exactly three permissions: list/watch on agentruns, update on
  agentruns/status, create on serviceaccounts/token restricted by
  `resourceNames` to `agent-reader`. Nothing else. Provider key comes from
  `spec.model.secretRef` as an env var; the rendered agent definition comes
  from a ConfigMap whose hash rolls the Deployment.
- **agent-reader** (one per permitted namespace): ServiceAccount, Role
  generated from `spec.permissions`, RoleBinding. Used only through
  ten-minute TokenRequest tokens minted by the runtime per run.
- **Approver**: whoever can update agentruns in that namespace. No new auth.
- The runtime never holds write access to workloads. This is the property the
  whole talk rests on. Do not weaken it for convenience.

### Runtime behaviour

- A controller-runtime process with one reconciler on AgentRun, label-selected
  to its own name, acting only on phase Accepted. Claims a run with a
  resourceVersion-guarded status update. Re-claims runs whose executor pod is
  gone, incrementing `attempts`, up to `maxAttempts`.
- Assembles the task: `instructions` in the system prompt, `objective` in the
  user message (untrusted input), namespace from `metadata.namespace`, fixed
  runtime rules and limits. `remediation.mode` never reaches the model.
- Tool loop: every tool result becomes an evidence candidate with an id; steps,
  tokens, and wall clock are enforced inside the loop. `finish()` carries
  diagnosis, evidence ids, and proposals. Every proposal must reference real
  evidence ids or the run fails as UngroundedProposal.
- Own loop in Go, roughly 500 lines, behind a one-method `Model` interface.
  Provider is a config switch. Ollama through the OpenAI-compatible endpoint is
  the in-cluster fallback. A fake model exists for envtest.
- Identical behaviour in `propose` and `auto` mode.

### Operator behaviour

- Admission: namespace permitted, agent Ready, budget available, concurrency
  below `maxConcurrentRuns`. Else Rejected at zero tokens. On Accepted, label
  the run with the agent name.
- On AwaitingApproval: compute digests over canonical action JSON, server-side
  dry-run each action, write both onto the proposals.
- Apply when `spec.approvals` digests match, or immediately in `auto` mode for
  types outside `approvalRequiredFor`. Recompute the digest before applying.
  Executors run in order and stop at the first failure. Then verify: wait for
  the Deployment's Available condition with a timeout.
- Action vocabulary (closed, one deterministic executor each):
  RollbackDeployment, SetImage, Scale, RestartRollout, SetEnv, SetResources,
  SetConfigMapKey, DeletePod, RawPatch. RawPatch is structurally never-auto and
  only targets allow-listed kinds: deployments, services, configmaps.
- Plans are straight lines. No observe-and-decide between writes. A follow-up
  is a new run.

### Triggers

- Alertmanager webhook is the only trigger. Firing creates a run with the
  objective rendered from the trigger template and alert annotations. Resolved
  supersedes the run if its phase is before Applying. The alert fingerprint is
  the dedup key: one open run per fingerprint, cooldown after terminal.
- Never watch Pods or Events for detection. Never let the model detect.

### Limits

- `maxSteps`, `maxTokensPerRun`, `timeout` are enforced. Cost is derived from
  usage and reported only, never enforced.

### Plugin

- `kubectl agent ask | approve | reject`. Nobody types a sha256 on stage.

### Demo

- kind on the presenter's Mac (M4, 48 GB). kube-prometheus-stack with custom
  PrometheusRules: `for: 20s`, scrape and evaluation interval 10s, Alertmanager
  `group_wait: 5s`, receiver with `send_resolved: true`. Break to Accepted run
  in under a minute. Preload images into kind.
- Three rehearsed failures, each grounded in state that still exists:
  bad image rollout (RollbackDeployment, previous ReplicaSet kept), renamed
  ConfigMap key (SetConfigMapKey then RestartRollout), Service selector
  mismatch (RawPatch with forced approval, entered through `kubectl agent ask`).
- Act 4 breaks the system deterministically: budget exhaustion, stale digest,
  action type outside the enum rejected by the API server. Prompt injection is
  shown as a recording, never live.

## Conventions

- Go 1.26, controller-runtime. Scaffolding tool is the author's choice and not
  yet decided; do not run one unless ordered.
- Every field that appears on a slide must be exercised in the demo. Do not add
  spec fields that the demo does not use.
- Prior art to cite and differentiate from, never to depend on: kagent
  (CNCF Sandbox), kubernetes-sigs/agent-sandbox.
