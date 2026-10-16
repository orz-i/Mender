import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Provider, ToolSummary } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { MemoryRouter } from "react-router";
import { afterEach, expect, it, vi } from "vitest";

import { CatalogPage } from "./catalog-page";

afterEach(cleanup);

const provider:Provider={
  providerId:"prov_1",namespace:"anysearch",displayName:"AnySearch",
  description:"Search online",createdAt:"2026-10-16T01:00:00Z",
  updatedAt:"2026-10-16T01:00:00Z",transport:"http",toolCount:105,
};
const tools:ToolSummary[]=Array.from({length:105},(_,i)=>({
  rank:i+1,toolId:"tool_"+String(i+1).padStart(4,"0"),
  providerId:provider.providerId,name:"search_"+(i+1),
  title:"Search tool "+(i+1),description:"Online search",
}));
function show(browseTools:ReturnType<typeof vi.fn>,searchTools=vi.fn()){
  const client={
    listProviders:vi.fn().mockResolvedValue({providers:[provider]}),
    browseTools,searchTools,
  } as unknown as MenderClient;
  render(<MemoryRouter><TooltipProvider><CatalogPage client={client} providerId={provider.providerId}/></TooltipProvider></MemoryRouter>);
}

it("shows the 101st service Tool after a bounded continuation, not client-side first-100 truncation",async()=>{
  const browse=vi.fn().mockResolvedValueOnce({tools:tools.slice(0,100),nextCursor:"browse-page-two"})
    .mockResolvedValueOnce({tools:tools.slice(100)});
  show(browse);
  expect(await screen.findByText("Search tool 1")).toBeTruthy();
  expect(screen.getByText("AnySearch")).toBeTruthy();
  expect(screen.getByRole("button",{name:"Load more tools"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Load more tools"}));
  await waitFor(()=>expect(browse).toHaveBeenCalledWith({
    limit:100,providerId:"prov_1",cursor:"browse-page-two",
  }));
  await waitFor(()=>expect(screen.queryByRole("button",{name:"Load more tools"})).toBeNull());
  fireEvent.change(screen.getByRole("combobox",{name:"Service tools rows per page"}),{target:{value:"50"}});
  fireEvent.click(screen.getByRole("button",{name:"Service tools next page"}));
  fireEvent.click(screen.getByRole("button",{name:"Service tools next page"}));
  expect(await screen.findByText("Search tool 101")).toBeTruthy();
  expect(screen.getByText("Search tool 105")).toBeTruthy();
});

it("retains exact Provider scope and query when extending lexical search",async()=>{
  const browse=vi.fn().mockResolvedValue({tools:[]});
  const search=vi.fn().mockResolvedValueOnce({tools:tools.slice(0,30),nextCursor:"search-page-two"})
    .mockResolvedValueOnce({tools:tools.slice(30,35)});
  show(browse,search);
  fireEvent.change(await screen.findByRole("textbox",{name:"Catalog search"}),{target:{value:"search"}});
  fireEvent.click(screen.getByRole("button",{name:"Search"}));
  expect(await screen.findByText("Search tool 1")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Load more tools"}));
  await waitFor(()=>expect(search).toHaveBeenCalledWith({
    query:"search",limit:30,providerId:"prov_1",cursor:"search-page-two",
  }));
});
