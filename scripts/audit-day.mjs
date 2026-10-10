#!/usr/bin/env node
// Read-only audit: no generated quotes, no insertion into the application database.
import { writeFile } from 'node:fs/promises';
const args=process.argv.slice(2);
const day=args[0];
if(!/^\d{4}-\d{2}-\d{2}$/.test(day??'')){console.error('Usage: node scripts/audit-day.mjs YYYY-MM-DD [base-url] [report.json]');process.exit(2)}
const base=args[1]??'http://127.0.0.1:8080';
const response=await fetch(`${base}/api/history/${day}`,{signal:AbortSignal.timeout(15000)});
if(!response.ok)throw new Error(`History request failed: ${response.status}`);
const review=await response.json();
const expected=[];
for(let m=570;m<=900;m++)if(m<=690||m>=780)expected.push(`${String(Math.floor(m/60)).padStart(2,'0')}:${String(m%60).padStart(2,'0')}`);
const observed=new Set();const issues=[];
const approx=(a,b)=>Number.isFinite(a)&&Number.isFinite(b)&&Math.abs(a-b)<0.000001;
for(const [i,d] of review.snapshots.entries()){
 const prefix=`snapshot ${i}`;
 if(d.day!==day)issues.push(`${prefix}: incorrect trading day`);
 if(!d.observed_at){issues.push(`${prefix}: source time missing`);continue;}
 const minute=d.observed_at.slice(11,16);
 if(observed.has(minute))issues.push(`${prefix}: duplicate source minute`);
 if(expected.includes(minute))observed.add(minute);
 if(!d.observed_at.startsWith(day)||!expected.includes(minute))issues.push(`${prefix}: outside session`);
 const age=(Date.parse(d.generated_at)-Date.parse(d.observed_at))/1000;
 if(!(age>=0&&age<=180))issues.push(`${prefix}: invalid source age`);
 if(d.sources.length<2||d.sources.some(s=>s.stale||s.error))issues.push(`${prefix}: invalid source status`);
 if(d.score.total!=null){
  if(d.score.factors.some(f=>f.score==null))issues.push(`${prefix}: score with missing factor`);
  else if(!approx(d.score.total,d.score.factors.reduce((sum,f)=>sum+f.score,0)))issues.push(`${prefix}: score sum differs`);
 }
 for(const key of ['sealing','capacity']){
  const s=d[key];if(!s)continue;
  if(!Number.isInteger(s.sealed)||!Number.isInteger(s.broken)||s.sealed<0||s.broken<0)issues.push(`${prefix}: ${key} invalid count`);
  const count=s.sealed+s.broken;
  if(count===0?s.rate!==null:!approx(s.rate,100*s.sealed/count))issues.push(`${prefix}: ${key} ratio differs`);
 }
 const b=d.breadth;
 if(!b||!d.sealing)issues.push(`${prefix}: required market metrics missing`);
 if(b&&Object.values({up:b.up,down:b.down,flat:b.flat,yang:b.yang,yin:b.yin,valid:b.valid}).some(n=>!Number.isInteger(n)||n<0))issues.push(`${prefix}: invalid breadth count`);
 if(b&&(b.up+b.down+b.flat!==b.valid||b.yang+b.yin!==b.valid))issues.push(`${prefix}: breadth counts differ`);
}
const missing=expected.filter(m=>!observed.has(m));
let longest=0,run=0;
for(const m of expected){run=observed.has(m)?0:run+1;longest=Math.max(longest,run);}
const closeValid=review.close?.day===day&&review.close?.observed_at?.startsWith(day)&&review.close.observed_at.slice(11,16)>='15:00'&&Number.isFinite(Date.parse(review.close.observed_at))&&Date.parse(review.close.observed_at)<=Date.parse(review.close.generated_at)&&review.close.sources.length>=2&&review.close.sources.every(s=>!s.error&&!s.stale);
if(!closeValid)issues.push('Valid closing snapshot is missing');
// Operational acceptance allows occasional single-minute misses, never missing an opening/closing window.
const coverage=observed.size/expected.length;
const firstWindow=expected.slice(0,3).some(m=>observed.has(m));
const lastWindow=expected.slice(-3).some(m=>observed.has(m));
const report={day,audited_at:new Date().toISOString(),snapshot_count:review.snapshots.length,expected_minutes:expected.length,observed_minutes:observed.size,coverage,missing_minutes:missing,longest_missing_run:longest,opening_observed:firstWindow,closing_observed:lastWindow,close_valid:!!closeValid,issues,passed:coverage>=0.95&&longest<=3&&firstWindow&&lastWindow&&!!closeValid&&issues.length===0};
const json=JSON.stringify(report,null,2)+'\n';
if(args[2])await writeFile(args[2],json);
console.log(json);process.exitCode=report.passed?0:1;
