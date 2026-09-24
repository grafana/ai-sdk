import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { streamText } from "../../tools/node_modules/ai/src/index.ts";

const directory=dirname(fileURLToPath(import.meta.url));
const parts=readFileSync(join(directory,"input.jsonl"),"utf8").trim().split("\n").map(line=>JSON.parse(line));
const model={specificationVersion:"v4",provider:"test",modelId:"test-model",supportedUrls:{},doStream:async()=>({stream:new ReadableStream({start(controller){for(const part of parts)controller.enqueue(part);controller.close();}})})};
const result=streamText({model:model as never,prompt:"test",maxRetries:0});
const lines:string[]=[];
for await(const chunk of result.toUIMessageStream({sendSources:true,generateMessageId:()=>"message-1"}))lines.push(JSON.stringify(chunk));
writeFileSync(join(directory,"expected.jsonl"),lines.join("\n")+"\n");
