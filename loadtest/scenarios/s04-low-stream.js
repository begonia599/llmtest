// S4: Low concurrency streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s4: {
      executor: 'constant-vus',
      vus: 10,
      duration: '120s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.95'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), true, 'medium', 0);
}
