# Superpowers Skill

## 概述

Superpowers 是一个增强型 AI 能力扩展包，为 Trae SOLO 提供强大的自动化和智能化能力。

## 能力特性

### 🧠 智能分析
- 深度代码分析与理解
- 需求拆解与规划
- 架构设计建议

### ⚡ 高效开发
- 自动代码生成与优化
- 智能调试与修复
- 测试用例自动生成

### 📝 文档能力
- 自动化文档生成
- 代码注释完善
- API 文档生成

### 🔧 工具集成
- Git 工作流优化
- CI/CD 配置自动化
- 项目配置管理

## 使用方法

### 自动调用
在 `agents.md` 中配置：
```markdown
<Skill name="superpowers" />
```

### 手动调用
```
<Skill name="superpowers" />
```

## 配置选项

```yaml
superpowers:
  enabled: true
  debug_mode: false
  max_context: 8192
```

## 版本
1.0.0

## 来源
https://github.com/obraz/superpowers
