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

## 实施与验收记录（2026-10-21）

### 基线缺陷确认

新增的 4 项数值及嵌套字段回归测试在代码修复前全部失败：不完整数字 1e 被清空，integer/range 错误未在字段旁显示，嵌套 URI 提示缺少控制关联，以及必填字段无就地错误。另有 1 项可选嵌套对象缺席时的误报 required 问题被单独复现并收口。基线证据：/tmp/mender-schema-fields-baseline-20261021.log、/tmp/mender-schema-optional-group-baseline.log。

### 分阶段代码交付

- d1a4233：文档先行，约束资源身份、配置语义、授权和测试行为。
- 34793df：数值输入使用保留原文的文本草稿、针对移动端的 numeric/decimal 键盘。词法错误、非有限数、不安全整数、Schema 类型/上下限错误不覆盖此前合法配置；合法值按原数字语义提交父组件。复用现有 JSON Schema 验证器显示字段级错误，增加 label、fieldset/legend、aria-required、aria-invalid 和 aria-describedby。多层对象不重复展示每层传递性错误；未设置的可选对象不虚假标红其子字段。提供必要的中文字段提示。
- ac4cf26：Provider Create 提交边界回归测试，证明中间态 1e 不会隐式发送旧配置，恢复为合法 4 后传递精确新值。
- 03013f0：高密度弹窗中焦点字段最小幅度滚动进入可见区域；字段因用户编辑而产生错误时，仅滚动当前焦点字段对应的错误，保留焦点及输入，不影响其他字段。

### 自动化与构建

ESLint、TypeScript、i18n、全量 Vitest 均通过，测试命令使用 --maxWorkers=6：60 个文件、376/376 项测试通过，1477/1477 条中文翻译覆盖。日志 /tmp/mender-schema-field-regression-20261021.log。

pnpm build 退出码 0，Web 和 Wails Desktop frontend 构建成功，日志 /tmp/mender-schema-field-build-20261021.log；原有 Vite 大分块提示非阻断。既有 Audit 重复 React key、Agent 测试路由与 Wails 浏览器预览消息不属于本轮改动。

### 隔离浏览器实测

使用本机现成 Chrome/Playwright，拦截全部 /api/v1/** 请求返回纯测试 Fixture，任何非 GET 请求均不可能进入真实后端。完整脚本 /tmp/mender-schema-field-visual-qa-20261021.mjs，结果 /tmp/mender-schema-field-qa-final-20261021.json。

1280×800 与 390×844 视口均确认：
- 可以点击 Endpoint 的实际 HTML label 聚焦输入控件；
- Endpoint 缺失时有字段级错误、必填和错误文本关联；
- 在已有有效 Attempts=3 的情形，编辑为不完整 1e 时保留原文字及原值，输入框报错，Save 禁用；
- 输入超过上限 11 时仍禁用并明确提示上限 10；改为合法 4 后 Save 恢复可用；
- 390px 文档宽 390px，弹窗宽 358px，Footer 底部 827px，不横向溢出；新错误能滚动至可见区、固定操作栏不遮挡；
- JavaScript pageerror 为 0，浏览器写入请求为 0。

截图：/tmp/mender-schema-fields-1280-incomplete.png、/tmp/mender-schema-fields-390-incomplete.png、/tmp/mender-schema-fields-390-valid.png。截图和结果来自 Web 隔离 Fixture，并非原生生产编辑器。

### Wails 及生产数据边界

Wails Dev 应用 PID 83099，新二进制时间为 2026-10-21 22:20:50。macOS AXUIElement 确认原生应用可正常访问「服务器」与「工具」，验证后导航已恢复至原先的「服务器」。本机当前未提供一个可以安全编辑的多层测试 Schema 插件，因此没有为验收新增或修改真实 Provider/Sandbox。此次新增源码可通过 Wails Dev/HMR 刷新，正式发行包需在后续打包时包含本轮提交。不能把 Web Fixture 测试描述为原生 Wails 表单提交通过。

只读生产 SQLite mode=ro 核对：providers=12、credentials=3、sandboxes=7、agent_tasks=75、agent_workspaces=0、policy_rules=0、PRAGMA quick_check=ok，与前期相同。没有新授权、没有执行 Agent Task、没有写入 Provider/Sandbox/Credential。原有四个未跟踪目录 .agents/、.commandcode/、.intent-loop/、.pi/ 仍保持原状。

遗留边界：真实原生多层 Schema、外部动态插件特有 UI Schema 映射、完整 VoiceOver、极深嵌套及可选对象显式添加/移除，需要在专门的非生产 Fixture 或已存在可安全修改的安装上进一步验收。此轮仅收口有证据的字段一致性、提示和焦点问题。
