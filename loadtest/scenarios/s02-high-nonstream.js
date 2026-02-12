// S2: High concurrency non-streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s2: {
      executor: 'constant-vus',
      vus: 100,
      duration: '60s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.90'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), false, 'fast', 0);
}
