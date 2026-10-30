# Domain Docs

工程类 skill 在探索本仓库代码时应如何消费领域文档。

## 探索之前，先读这些

- **`GLOSSARY.md`**（仓库根目录），或
- **`GLOSSARY-MAP.md`**（仓库根目录，如存在）：它指向每个 context 的 `GLOSSARY.md`，读其中与当前话题相关的那些。
- **`docs/adr/`**：读与即将改动的区域相关的 ADR。多 context 仓库还要检查 `src/<context>/docs/adr/` 里的 context 级决策。

**这些文件不存在就静默跳过。** 不要提示它们缺失，也不要建议提前创建。`/domain-modeling` skill（经由 `/grill-with-docs` 与 `/improve-codebase-architecture` 触达）会在术语或决策真正被敲定时才惰性创建它们。

## 文件结构

单 context 仓库（绝大多数仓库，**本仓库属于此类**）：

```
/
├── GLOSSARY.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── ...
```

多 context 仓库（根目录存在 `GLOSSARY-MAP.md` 为标志）：

```
/
├── GLOSSARY-MAP.md
├── docs/adr/                          ← 系统级决策
└── src/
    ├── ordering/
    │   ├── GLOSSARY.md
    │   └── docs/adr/                  ← context 专属决策
    └── billing/
        ├── GLOSSARY.md
        └── docs/adr/
```

## 使用 glossary 的词汇

当产出里出现领域概念（issue 标题、重构提案、假设、测试名）时，使用 `GLOSSARY.md` 定义的术语。不要漂移到 glossary 明确弃用的同义词。

如果你需要的概念还不在 glossary 里，这是个信号：要么你在发明项目并不使用的语言（重新考虑），要么存在真实空缺（记下来交给 `/domain-modeling`）。

## 标出 ADR 冲突

如果你的产出与既有 ADR 矛盾，明确指出来，不要静默覆盖：

> _与 ADR-0007（event-sourced orders）冲突，但值得重开，因为…_
