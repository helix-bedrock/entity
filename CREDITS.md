# Credits

Code in this repo that was ported from other projects, as required by the
[porting rules](https://github.com/helix-bedrock/.github/blob/main/PORTING.md).

| Helix path | Source project | Source path | Commit | License |
|---|---|---|---|---|
| `behavior/stdsort.go` | [LLVM libc++](https://github.com/llvm/llvm-project) | `libcxx/include/__algorithm/sort.h` (`std::sort`: introsort, partitions, insertion and heap sorts) | not pinned; checked step by step against the copy compiled into BDS 1.26.60 | Apache-2.0 WITH LLVM-exception |
