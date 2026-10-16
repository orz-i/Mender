import {useEffect,useState} from "react";
import type {MenderClient,Provider} from "@mender/sdk";
import {useNavigate,useParams} from "react-router";
import {Button} from "@mender/ui/components/button";
import {ErrorNotice} from "../../shared/page";
import {PageToolbar,ResultPanel} from "../../shared/page-toolbar";
import {t} from "../../i18n/i18n";
import {errorMessage} from "../../shared/errors";
import {WorkspaceTargetSettings} from "./workspace-targets";

export function ToolWorkspacePage({client}:{client:MenderClient}){
 const {providerId}=useParams();
 const navigate=useNavigate();
 const [provider,setProvider]=useState<Provider|null>(null);
 const [error,setError]=useState("");
 const [loaded,setLoaded]=useState(false);
 useEffect(()=>{
  if(!providerId)return;
  let live=true;
  void client.listProviders().then(({providers})=>{
   if(live){setProvider(providers.find(p=>p.providerId===providerId&&p.toolSourceKind==="sandbox"&&p.pluginId==="io.mender.provider.workspace")??null);setLoaded(true);}
  }).catch((e:unknown)=>{if(live){setError(errorMessage(e));setLoaded(true);}});
  return ()=>{live=false;};
 },[client,providerId]);
 return <>
  <PageToolbar actions={<Button size="sm" variant="outline" onClick={()=>void navigate(providerId?"/tools/"+providerId:"/tools")}>{t("Back to service tools")}</Button>}/>
  {error?<ErrorNotice message={error}/>:null}
  {loaded&&providerId&&provider?.toolSourceId?<WorkspaceTargetSettings client={client} providerId={providerId} defaultSandboxId={provider.toolSourceId}/>:
    <ResultPanel title="Workspace targets"><p className="p-6 text-sm text-muted-foreground">{t(loaded?"This service does not provide Sandbox Workspace Tools.":"Loading services…")}</p></ResultPanel>}
 </>;
}
