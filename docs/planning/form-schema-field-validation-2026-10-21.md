# Mender 表单治理：动态 Schema 数值草稿与字段校验（2026-10-21）

## 目标与实机范围

接续 `form-generic-config-integrity-2026-10-21.md`，本轮只针对已供应的动态 Schema 表单做字段级校验与焦点审计。仓库初始 HEAD `829372a`；Wails Dev 新构建进程 PID 83099，二进制时间 2026-10-21 22:20:50。未在生产 Provider/Sandbox 做测试配置写入。

源码证据：
- `generic-provider-config-form.tsx` 中 number/integer 使用 `<Input type="number">`，`onChange` 将浏览器提供的 raw 字符串解析成 Number，非有限值或空字串转成 `undefined` 并写回父级。Chrome number 控件对 `1e`、`-` 等输入中间态可能清洗为 `""`，从而把先前有效的可选值错误清除，Schema 校验可能允许 Save。
- `generic-provider-config-schema.ts` 已完整支持 integer、minimum、maximum、string format/length/pattern 以及递归 nested required；但 `FieldShell` 为视觉上的 div 标签，控件只靠 `aria-label`，缺少可点击/聚焦的关联和本地错误指示；底部统一错误在长弹窗里可能离字段很远。
- 同一动态表单用于 Provider Create/Edit/Rebind 和外部 Sandbox Create。必须保护所有入口，沿用上阶段已建立的 `onDraftErrorChange` 门禁与 schema 校验。

## 不变量

1. 未完成的数值输入（例如 `-`、`1e`）按原样留在编辑器；不得悄悄清空/更改上一个合法配置，也不得把旧配置提交。
2. `integer`、`minimum`、`maximum` 和数值异常应就地报错；合法输入恢复父级同步，百分比/单位等不得暗自转换；客户端不能接受非有限数、不安全整数或不属于 Schema 支持范围的值。
3. Required 缺失、字符串格式/范围/模式和 JSON 数组子项问题应在相关字段附近说明；复杂 Schema 的根级验证仍保留，不能删后端验证或放宽条件。
4. 输入标签在键盘、鼠标、辅助功能系统中均可关联字段；`aria-invalid` 与 `aria-describedby` 指向实际可见错误。错误与提示不抢占输入焦点；应保持固定 Footer、正文滚动。
5. 嵌套对象继续使用现有递归 Schema 与控件，不新增一套表单、模态向导、通用复杂 Schema 引擎、兼容层或数据持久化。
6. 外部插件提供的 Schema/UI hints 仅作为描述，不授予新权限、不生成/回显凭据秘密；Provider/Sandbox API、Go、MCP、ACP 与管理状态不变。
7. 不写入真实 Provider、Credential、Sandbox、Agent Task；测试使用本地 React fixture、浏览器网络拦截和只读 Wails 观察。分阶段提交，保留四个未跟踪工具目录。

## 阶段与判定

- E0 文档与基线：书面限定问题与非变量；先用 Vitest 复现数值中间态误删、整数/范围/Required/URI 缺少字段级反馈、点击标签无法聚焦；保留失败证据。
- E1 修复：复用已成熟的草稿状态及原 `genericConfigValidationError`，实现受控数值字符串、就地验证、语义标签和错误关联。验证字段父级变化时仅合理同步。
- E2 定向集成：嵌套动态 Schema 及 Provider Create/External Sandbox Review 按原有门禁执行，校验提交对象不会因无效草稿恢复旧值。
- E3 质量/视觉：TS、ESLint、i18n、Vitest、Web/Desktop 构建；隔离浏览器 1280×800/390×844。若生产缺少可安全浏览的安装测试 Schema，只做 Wails 只读页面核对，不制造数据。
- E4 文档与分段提交收口：标记原生实测/浏览器 fixture 与未覆盖路径，保存截图和测试结果。

设计参考：W3C WCAG 2.2 [Error Identification](https://www.w3.org/WAI/WCAG22/Understanding/error-identification)、[Labels or Instructions](https://www.w3.org/WAI/WCAG22/Understanding/labels-or-instructions)、GOV.UK [Error message](https://design-system.service.gov.uk/components/error-message/)、React [Controlled input](https://react.dev/reference/react-dom/components/input)。
