# Issue tracker: GitHub

本仓库的 issue 与 spec 存放在 GitHub Issues，全部操作使用 `gh` CLI。

## 仓库与归属规则

- 目标仓库：`praise579/subs-check`（`beck-8/subs-check` 的 **fork**，默认分支 `master`）。
- 只向 `praise579/subs-check` 提 issue。**不要**向上游 `beck-8/subs-check` 提 issue，除非用户明确要求。
- 这是 fork 仓库，`gh` 可能默认解析到上游父仓库。每次会话的第一次 `gh issue create` 之前，先用 `gh repo view --json nameWithOwner` 确认目标是 `praise579/subs-check`；若解析成上游，先向用户确认再继续。
- `gh` 在克隆目录内会自动从 `git remote -v` 推断仓库；本仓库 `origin` 为 `git@github.com:praise579/subs-check.git`。

## Conventions

- **建 issue**：`gh issue create --title "..." --body "..."`。多行正文用 heredoc。
- **读 issue**：`gh issue view <number> --json number,title,body,labels,comments`。
- **列 issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，按需加 `--label` 与 `--state` 过滤。
- **把 issue 设为某个 issue 的子 issue**：`gh issue create --parent <parent> ...`，或事后 `gh issue edit <parent> --add-sub-issue <child>`（`gh` 2.94+）。旧版 `gh`：`gh api --method POST repos/<owner>/<repo>/issues/<parent>/sub_issues -F sub_issue_id=<child-db-id>`（用 database id，见下方 **Blocking**）。若 sub-issue 不可用，在子 issue 正文顶部写 `Part of #<parent>`。
- **Blocking**：用 GitHub 原生 issue 依赖，这是唯一在 UI 中可见的规范表示。加边：`gh issue create --blocked-by <n>,<n> ...` 或 `gh issue edit <child> --add-blocked-by <blocker>`（`gh` 2.94+）。旧版 `gh`：`gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`，其中 `<blocker-db-id>` 是阻塞者的数字 **database id**（`gh api repos/<owner>/<repo>/issues/<n> --jq .id`，**不是** `#number` 也不是 `node_id`）。依赖功能不可用时，退化为在子 issue 正文顶部写 `Blocked by: #<n>, #<n>`。所有阻塞者关闭后该 ticket 即解锁。
- **统计未关闭阻塞者**：`gh api repos/<owner>/<repo>/issues/<n> --jq .issue_dependencies_summary.blocked_by`（只算未关闭者，0 表示可以开工）。`gh issue view <n> --json blockedBy` 也会列出已关闭的阻塞者，需过滤：`--jq '[.blockedBy.nodes[] | select(.state == "OPEN")] | length'`。
- **评论**：`gh issue comment <number> --body "..."`。
- **加 / 去标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`。
- **关闭**：先用 `gh issue comment <number> --body "..."` 说明原因，再 `gh issue close <number>`。对已关闭的 issue 执行 `gh issue close --comment` 会丢掉评论但仍然退出码 0。

## Pull requests as a triage surface

**PRs as a request surface: no.** _(若本仓库把外部 PR 也视作需求提交，改为 `yes`；`/triage` 会读这个开关。)_

设为 `yes` 时，PR 与 issue 走同一套标签与状态，命令换成 `gh pr` 等价物：

- **读 PR**：`gh pr view <number>`，再 `gh pr view <number> --comments`（非 TTY 下 `--comments` 只打评论、不含正文），diff 用 `gh pr diff <number>`。
- **列出待 triage 的外部 PR**：`gh api --paginate 'repos/{owner}/{repo}/pulls?state=open' --jq '.[] | select(.author_association | IN("OWNER","MEMBER","COLLABORATOR") | not) | {number, title, author: .user.login, author_association, labels: [.labels[].name]}'`。
- **评论 / 打标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label` / `--remove-label`、`gh pr close`。

GitHub 的 issue 与 PR 共用一套编号，裸写 `#42` 可能是两者之一：先用 `gh pr view 42` 解析，失败再退回 `gh issue view 42`。

## When a skill says "publish to the issue tracker"

创建一个 GitHub issue。

## When a skill says "fetch the relevant ticket"

按上面 **读 issue** 的方式读取。

## Wayfinding operations

供 `/wayfinder` 使用。**map** 是一个 issue，**child** issue 是其中的 ticket。

- **Map**：单个 issue，标签 `wayfinder:map`，正文承载 Notes / Decisions-so-far / Fog。`gh issue create --label wayfinder:map`。
- **Child ticket**：以 GitHub sub-issue 形式挂到 map 上（见 **把 issue 设为某个 issue 的子 issue**）。sub-issue 不可用时，把 child 加进 map 正文的任务列表，并在 child 正文顶部写 `Part of #<map>`。标签：`wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。被认领后 assign 给执行者。
- **Blocking**：见 Conventions 下的 **Blocking**。
- **Frontier 查询**：列出 map 的未关闭 child（`gh issue list --state open`，范围限定为 map 的 sub-issue / 任务列表），剔除有未关闭阻塞者（见 **统计未关闭阻塞者**，或 `Blocked by` 行里未关闭的 issue）或已有 assignee 的，按 map 顺序取第一个。
- **认领**：`gh issue edit <n> --add-assignee @me`，这是本会话的第一次写操作。
- **收尾**：`gh issue comment <n> --body "<答案>"`，然后 `gh issue close <n>`，最后把上下文指针（gist + 链接）追加到 map 的 Decisions-so-far。
