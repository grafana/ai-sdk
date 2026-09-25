// Synthetic deterministic integration upstreams, not provider recordings.
import assert from 'node:assert/strict';
import {execFileSync, spawn} from 'node:child_process';
import {mkdtempSync, writeFileSync, rmSync} from 'node:fs';
import {createServer} from 'node:http';
import {createServer as createNetServer} from 'node:net';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {test} from 'node:test';
import OpenAI from 'openai';

async function port(): Promise<number> {
  const server=createNetServer(); await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));
  const address=server.address(); assert.ok(address && typeof address!=='string');
  await new Promise<void>(r=>server.close(()=>r())); return address.port;
}
function token(): string {
  const encode=(value: unknown)=>Buffer.from(JSON.stringify(value)).toString('base64url');
  return `${encode({alg:'ES256',typ:'at+jwt'})}.${encode({sub:'access-policy:adapter',aud:['ai-sdk'],exp:Math.floor(Date.now()/1000)+3600,namespace:'stack-adapter',serviceIdentity:'adapter-test'})}.${Buffer.alloc(64).toString('base64url')}`;
}

test('official JavaScript SDK against real command: compatible and Anthropic profiles', async t=>{
  const directory=mkdtempSync(join(tmpdir(),'chat-completions-adapter-js-'));
  const binary=join(directory,'gateway');
  try {
    execFileSync('go',['build','-race','-mod=readonly','-o',binary,'./cmd/grafana-ai-gateway'],{cwd:resolve(import.meta.dirname,'../..'),env:{...process.env,GOWORK:'off'},stdio:'pipe'});
    for (const backend of ['openai-compatible','anthropic'] as const) {
      await t.test(backend,async()=>{
        const requests: Record<string,any>[]=[];
        let failNext=false;
        const upstream=createServer(async(req,res)=>{
          const chunks: Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));
          const body=JSON.parse(Buffer.concat(chunks).toString());requests.push(body);
          if(failNext){failNext=false;res.writeHead(503,{'Content-Type':'application/json'});res.end(JSON.stringify({type:'error',error:{type:'overloaded_error',message:'synthetic retryable'}}));return;}
          const isAnthropic=backend==='anthropic';
          if(!isAnthropic)assert.equal(body.store,false);
          assert.equal(req.url,isAnthropic?'/v1/messages?beta=true':'/v1/chat/completions');
          assert.equal(isAnthropic?req.headers['x-api-key']:req.headers.authorization,isAnthropic?'fake-key':'Bearer fake-key');
          const tool=!!body.tools?.length && !JSON.stringify(body.messages).includes('tool_result') && !body.messages.some((m:any)=>m.role==='tool');
          if(isAnthropic){
            const content=tool?[{type:'tool_use',id:'call_weather',name:'weather',input:{city:'Rio'}}]:[{type:'text',text:'hello'}];
            const message={id:'msg_private',type:'message',role:'assistant',model:body.model,content,stop_reason:tool?'tool_use':'end_turn',stop_sequence:null,usage:{input_tokens:3,output_tokens:2}};
            if(!body.stream){res.setHeader('Content-Type','application/json');res.end(JSON.stringify(message));return;}
            res.setHeader('Content-Type','text/event-stream');
            const events:any[]=[{type:'message_start',message:{...message,content:[],stop_reason:null,usage:{input_tokens:3,output_tokens:0}}}];
            if(tool){events.push({type:'content_block_start',index:0,content_block:{type:'tool_use',id:'call_weather',name:'weather',input:{}}},{type:'content_block_delta',index:0,delta:{type:'input_json_delta',partial_json:'{"city":"Rio"}'}})}
            else{events.push({type:'content_block_start',index:0,content_block:{type:'text',text:''}},{type:'content_block_delta',index:0,delta:{type:'text_delta',text:'hello'}})}
            events.push({type:'content_block_stop',index:0},{type:'message_delta',delta:{stop_reason:message.stop_reason,stop_sequence:null},usage:{output_tokens:2}},{type:'message_stop'});
            for(const event of events)res.write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);res.end();return;
          }
          const call={id:'call_weather',type:'function',function:{name:'weather',arguments:'{"city":"Rio"}'}};
          const message=tool?{role:'assistant',content:null,tool_calls:[call]}:{role:'assistant',content:'hello'};
          const finish=tool?'tool_calls':'stop';
          const base={id:'private',created:1,model:body.model};
          if(!body.stream){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({...base,object:'chat.completion',choices:[{index:0,message,finish_reason:finish}],usage:{prompt_tokens:3,completion_tokens:2,total_tokens:5}}));return;}
          res.setHeader('Content-Type','text/event-stream');
          const delta=tool?{tool_calls:[{...call,index:0}]}:{content:'hello'};
          for(const choice of [{index:0,delta,finish_reason:null},{index:0,delta:{},finish_reason:finish}])res.write(`data: ${JSON.stringify({...base,object:'chat.completion.chunk',choices:[choice]})}\n\n`);
          res.write(`data: ${JSON.stringify({...base,object:'chat.completion.chunk',choices:[],usage:{prompt_tokens:3,completion_tokens:2,total_tokens:5}})}\n\ndata: [DONE]\n\n`);res.end();
        });
        await new Promise<void>(r=>upstream.listen(0,'127.0.0.1',r));const addr=upstream.address();assert.ok(addr && typeof addr!=='string');
        const address=`127.0.0.1:${await port()}`;const baseURL=`http://127.0.0.1:${addr.port}${backend==='anthropic'?'':'/v1'}`;
        const config=join(directory,`${backend}.yaml`);
        const fallbackConfig=backend==='anthropic'?'  public/fallback:\n    name: Fallback\n    primary:\n      provider: local\n      model: claude-sonnet-4-20250514\n    fallback:\n      - provider: local\n        model: claude-3-5-haiku-20241022\n':'';
        writeFileSync(config,`providers:\n  local:\n    type: ${backend}\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_JS_KEY\n    baseURL: ${baseURL}\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: local\n      model: ${backend==='anthropic'?'claude-sonnet-4-20250514':'gpt-4o-mini'}\n    aliases: [chat]\n${fallbackConfig}`);
        const env=Object.fromEntries(Object.entries(process.env).filter(([key])=>!['GRAFANA_AI_GATEWAY_','AGENTO11Y_','SIGIL_'].some(prefix=>key.startsWith(prefix))));
        const proc=spawn(binary,[`--config.file=${config}`,'--deployment.mode=development','--auth.unsafe',`--server.listen-address=${address}`,'--server.shutdown-timeout=2s'],{env:{...env,CHAT_COMPLETIONS_ADAPTER_JS_KEY:'fake-key'},stdio:['ignore','ignore','pipe']});
        let stderr='';proc.stderr.on('data',chunk=>stderr+=chunk);const exited=new Promise<number|null>(r=>proc.once('exit',r));
        try {
          let ready=false;for(let i=0;i<400;i++){try{const response=await fetch(`http://${address}/ready`);ready=response.ok;await response.text();if(ready)break}catch{}await new Promise(r=>setTimeout(r,25))}assert.ok(ready,stderr);
          const client=new OpenAI({apiKey:token(),baseURL:`http://${address}/v1`,maxRetries:0});
          const messages: OpenAI.Chat.Completions.ChatCompletionMessageParam[]=[{role:'user',content:'hello'}];
          const unary=await client.chat.completions.create({model:'chat',messages});assert.equal(unary.model,'public/chat');assert.equal(unary.choices[0].message.content,'hello');assert.equal(unary.usage?.total_tokens,5);
          const tools: OpenAI.Chat.Completions.ChatCompletionTool[]=[{type:'function',function:{name:'weather',parameters:{type:'object',properties:{city:{type:'string'}},required:['city']}}}];
          const called=await client.chat.completions.create({model:'chat',messages,tools});assert.equal(called.choices[0].finish_reason,'tool_calls');assert.equal(called.choices[0].message.tool_calls?.[0].id,'call_weather');
          const continued=await client.chat.completions.create({model:'chat',messages:[...messages,called.choices[0].message,{role:'tool',tool_call_id:'call_weather',content:'sunny'}],tools});assert.equal(continued.choices[0].message.content,'hello');
          for(const withTools of [false,true]){
            const stream=await client.chat.completions.create({model:'chat',messages,...(withTools?{tools}:{}),stream:true,stream_options:{include_usage:true}});let text='',args='',finishes=0,usage=0;
            for await(const chunk of stream){if(!chunk.choices.length)usage=chunk.usage?.total_tokens??0;for(const choice of chunk.choices){text+=choice.delta.content??'';for(const call of choice.delta.tool_calls??[])args+=call.function?.arguments??'';if(choice.finish_reason)finishes++}}
            assert.equal(finishes,1);assert.equal(usage,5);if(withTools)assert.deepEqual(JSON.parse(args),{city:'Rio'});else assert.equal(text,'hello');
          }
          const before=requests.length;await assert.rejects(()=>client.chat.completions.create({model:'chat',messages,n:2}),error=>error instanceof OpenAI.APIError && error.status===400);assert.equal(requests.length,before);
          if(backend==='anthropic'){
            failNext=true;const count=requests.length;const recovered=await client.chat.completions.create({model:'public/fallback',messages});assert.equal(recovered.model,'public/fallback');assert.equal(recovered.choices[0].message.content,'hello');assert.equal(requests.length,count+2);
            const after=requests.length;await assert.rejects(()=>client.chat.completions.create({model:'public/fallback',messages,tools}),error=>error instanceof OpenAI.APIError && error.status===400);assert.equal(requests.length,after);
          }
          assert.ok(!JSON.stringify(unary).includes('private'));
        } finally {
          proc.kill('SIGINT');const timer=setTimeout(()=>proc.kill('SIGKILL'),5000);const code=await exited;clearTimeout(timer);assert.equal(code,0,stderr);upstream.closeAllConnections();await new Promise<void>(r=>upstream.close(()=>r()));
        }
      });
    }
  } finally {rmSync(directory,{recursive:true,force:true});}
});
