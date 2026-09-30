// Deterministic local performance fixture. Never included in the production bundle.
import http from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, extname } from 'node:path'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('../dist/', import.meta.url))
const files = Array.from({length:200},(_,i)=>({path:'/synthetic/meeting-'+String(i).padStart(3,'0')+'.txt',name:'meeting-'+String(i).padStart(3,'0')+'.txt'}))
const jobs = Array.from({length:1000},(_,i)=>({
 id:'fixture-'+i,kind:'minutes',path:files[Math.max(0,i-800)]?.path || '/synthetic/history-'+i+'.txt',
 name:i>=800?files[i-800].name:'history-'+i+'.txt',
 status:i<800?'completed':i===999?'running':'queued',phase:i===999?'Codex가 회의록을 작성 중':'대기 중',
 model:'gpt-5.6-sol',effort:'low',result:'',recentOutput:'',prompt:'',outputPath:'',error:'',createdAt:'2026-09-30T00:00:00Z'
}))
const longText=Array.from({length:1600},(_,i)=>'합성 회의 자료 '+i+' — 미리보기의 높이와 스크롤을 검증합니다.').join('\n').slice(0,64000)
const snapshot=()=>({jobs:jobs.slice(-250),codexReady:true,whisperReady:true,whisperInstalling:false,whisperError:''})
const clients=new Set()
const timer=setInterval(()=>{
 const live=jobs[999]
 live.result=(live.result+'합성 결과 문장입니다.\n').slice(-120000)
 live.recentOutput=(live.recentOutput+'합성 출력\n').slice(-30000)
 for(const response of clients)response.write('event: state\ndata: '+JSON.stringify(snapshot())+'\n\n')
},200)
const mime={'.html':'text/html','.js':'text/javascript','.css':'text/css','.woff2':'font/woff2'}
const server=http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://127.0.0.1')
 const json=value=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(value))}
 if(url.pathname==='/fixture/cart')return json(files)
 if(url.pathname==='/api/events'){res.setHeader('Content-Type','text/event-stream');res.write('event: state\ndata: '+JSON.stringify(snapshot())+'\n\n');clients.add(res);req.on('close',()=>clients.delete(res));return}
 if(url.pathname==='/api/state')return json(snapshot())
 if(url.pathname==='/api/config')return json({listen:'127.0.0.1:8792',codexBinary:'synthetic',ffmpegBinary:'synthetic',outputDir:'',whisperModel:'base',prompt:'합성 테스트 회의록 프롬프트'})
 if(url.pathname==='/api/models')return json({models:[{id:'gpt-5.6-sol',model:'gpt-5.6-sol',displayName:'Sol',supportedReasoningEfforts:[{reasoningEffort:'low'}]}]})
 if(url.pathname==='/api/preview')return json({text:longText,transcribed:false})
 if(url.pathname==='/api/environment')return json({codex:{path:'synthetic',ready:true},whisper:{model:'base',modelPath:'synthetic',modelReady:true,binaryPath:'synthetic',binaryReady:true,installerVersion:'fixture'},ffmpeg:{path:'synthetic',ready:true,version:'fixture'}})
 if(url.pathname.startsWith('/api/jobs/')&&url.pathname.endsWith('/cancel')){const job=jobs.find(j=>j.id===url.pathname.split('/')[3]);if(job){job.status='cancelled';job.phase='취소됨'}return json(snapshot())}
 if(url.pathname.startsWith('/api/jobs/')){const job=jobs.find(j=>j.id===url.pathname.split('/').pop());return json({...job,prompt:'합성 테스트 프롬프트'})}
 if(url.pathname.startsWith('/api/')){res.statusCode=404;return json({error:'Fixture endpoint unavailable'})}
 const path=resolve(root,'.'+(url.pathname==='/'?'/index.html':url.pathname))
 if(!path.startsWith(root)){res.statusCode=403;return res.end()}
 try{const data=await readFile(path);res.setHeader('Content-Type',mime[extname(path)]||'application/octet-stream');res.end(data)}catch{res.statusCode=404;res.end()}
})
server.listen(8792,'127.0.0.1',()=>console.log('Synthetic performance fixture: http://127.0.0.1:8792'))
for(const signal of ['SIGTERM','SIGINT'])process.on(signal,()=>{clearInterval(timer);for(const res of clients)res.end();server.close()})
