// S8: High error rate stress test
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s8: {
      executor: 'constant-vus',
      vus: 100,
      duration: '120s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.50'], // Lower threshold due to high error rate
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), true, 'medium', 0.20);
}
