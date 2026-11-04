import { CHAPTERS, RUN } from '../src/story.js';
import { GENERATED } from '../src/timeline.generated.js';

const problems = [];
const ONES = ['zero', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten', 'eleven', 'twelve', 'thirteen', 'fourteen', 'fifteen', 'sixteen', 'seventeen', 'eighteen', 'nineteen'];
const TENS = ['', '', 'twenty', 'thirty', 'forty', 'fifty', 'sixty', 'seventy', 'eighty', 'ninety'];
const words = (n) => (n < 20 ? ONES[n] : `${TENS[Math.floor(n / 10)]}${n % 10 ? `-${ONES[n % 10]}` : ''}`);

const rendered = new Map(GENERATED.captions.map((c) => [`${c.chapter}:${c.i}`, c.text]));
for (const chapter of CHAPTERS) {
  chapter.lines.forEach((line, i) => {
    if (rendered.get(`${chapter.id}:${i}`) !== line.text) problems.push(`narration ${chapter.id}[${i}] differs from the rendered audio: "${line.text}"`);
  });
  const extra = GENERATED.captions.filter((c) => c.chapter === chapter.id).length - chapter.lines.length;
  if (extra) problems.push(`chapter ${chapter.id} has ${extra} more rendered sentences than the script`);
}

const spoken = CHAPTERS.map((c) => c.narration.toLowerCase()).join(' ');
const claims = [[RUN.events.total, 'events'], [RUN.versions.length, 'versions'], [RUN.run.rows.episodes, 'episode']];
for (const [n, noun] of claims) if (!spoken.includes(`${words(n)} ${noun}`)) problems.push(`the narration never says "${words(n)} ${noun}", but the run has ${n}`);

console.log(`run ${RUN.run.id} (id from the ${RUN.run.idFrom}), final version ${RUN.run.finalVersion}`);
if (problems.length) { console.error(problems.join('\n')); process.exit(1); }
console.log('narration, rendered audio and run data agree');
