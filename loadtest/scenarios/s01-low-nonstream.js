// S1: Low concurrency non-streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s1: {
      executor: 'constant-vus',
      vus: 10,
      duration: '60s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.95'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), false, 'fast', 0);
}
