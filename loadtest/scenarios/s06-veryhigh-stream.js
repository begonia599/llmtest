// S6: Very high concurrency streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s6: {
      executor: 'constant-vus',
      vus: 500,
      duration: '120s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.80'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), true, 'medium', 0);
}
