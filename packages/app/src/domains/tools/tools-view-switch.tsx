import { FocusSwitch } from "../../shared/focus-switch";

export type ToolsView = "catalog" | "sources";

export function ToolsViewSwitch({
  view,
  onChange,
}: {
  view: ToolsView;
  onChange: (view: ToolsView) => void;
}) {
  return <FocusSwitch
    label="Tools view"
    value={view}
    items={[
      { value: "catalog", label: "Tool catalog" },
      { value: "sources", label: "Tool sources" },
    ]}
    onChange={onChange}
  />;
}
