// S5: High concurrency streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s5: {
      executor: 'constant-vus',
      vus: 100,
      duration: '120s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.90'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), true, 'medium', 0);
}
