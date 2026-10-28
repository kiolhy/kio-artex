#!/usr/bin/env python3
"""settings_contract.py —— 检查 settings 表的键有没有"只读不写"的漂移。

为什么值得单独做个检查：`GetSetting("foo")` 拼错一个字母不会报错，只会永远返回
空值，于是功能静默失效（典型症状："开关打开没效果""默认值不生效"）。这类 bug 在
代码里看不出来，但用脚本扫一遍就能发现：**有读、没写**的键一定是错的（要么拼错，
要么写入那行被删了）。

检查规则：
  1. 收集所有 GetSetting("<字面量>") 与 SetSetting("<字面量>", ...) 的键；
  2. 读了但从未写过的键 → 错误（退出码 1）；
  3. 写了但从未读过的键 → 提示（可能是给前端用的，不算错）；
  4. 非字面量（变量/拼接）→ 提示，人工确认。

用法：
  python3 scripts/settings_contract.py            # 全仓库
  python3 scripts/settings_contract.py --strict   # 提示也当错误
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

# 只扫业务代码：norma 是 vendored SDK，android 是另一个项目。
SCAN_DIRS = ["db", "server", "agent", "intercept", "guard", "traffic", "llmrec", "notify", "evidence", "config", "cmd"]

READ_RE = re.compile(r'\bGetSetting\(\s*"([^"]+)"')
WRITE_RE = re.compile(r'\bSetSetting\(\s*"([^"]+)"')
DYN_READ_RE = re.compile(r"\bGetSetting\(\s*[^\")\s]")
DYN_WRITE_RE = re.compile(r"\bSetSetting\(\s*[^\")\s]")


def go_files(root: Path) -> list[Path]:
    out: list[Path] = []
    for d in SCAN_DIRS:
        base = root / d
        if not base.is_dir():
            continue
        out.extend(p for p in base.rglob("*.go") if not p.name.endswith("_test.go"))
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description="检查 settings 键的读写一致性")
    ap.add_argument("--strict", action="store_true", help="写了没读也当错误")
    ap.add_argument("--root", default=".", help="仓库根目录")
    args = ap.parse_args()

    root = Path(args.root).resolve()
    reads: dict[str, list[str]] = {}
    writes: dict[str, list[str]] = {}
    dynamic: list[str] = []

    files = go_files(root)
    if not files:
        print("没有找到任何 Go 源文件，检查路径是否正确", file=sys.stderr)
        return 2

    for f in files:
        rel = f.relative_to(root).as_posix()
        text = f.read_text(encoding="utf-8", errors="replace")
        for m in READ_RE.finditer(text):
            reads.setdefault(m.group(1), []).append(rel)
        for m in WRITE_RE.finditer(text):
            writes.setdefault(m.group(1), []).append(rel)
        if DYN_READ_RE.search(text):
            dynamic.append(f"{rel}: 存在非字面量的 GetSetting 调用")

    read_only = sorted(k for k in reads if k not in writes)
    write_only = sorted(k for k in writes if k not in reads)

    print(f"扫描 {len(files)} 个文件：读 {len(reads)} 个键，写 {len(writes)} 个键\n")

    failed = False
    if read_only:
        failed = True
        print("✗ 只读不写（拼错或写入被删，功能会静默失效）：")
        for k in read_only:
            print(f"    {k}  ← {', '.join(sorted(set(reads[k])))}")
        print()

    if write_only:
        mark = "✗" if args.strict else "·"
        print(f"{mark} 写了没读（可能给前端用，人工确认）：")
        for k in write_only:
            print(f"    {k}  ← {', '.join(sorted(set(writes[k])))}")
        print()
        if args.strict:
            failed = True

    if dynamic:
        print("· 非字面量调用（无法静态确认，需人工看）：")
        for d in dynamic:
            print(f"    {d}")
        print()

    if failed:
        print("结论：存在只读不写的 settings 键，请修正键名或补上写入。")
        return 1
    print("结论：settings 键读写一致。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
