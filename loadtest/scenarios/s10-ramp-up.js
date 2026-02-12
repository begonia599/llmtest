// S10: Ramp-up test (10 -> 500 VUs)
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s10: {
      executor: 'ramping-vus',
      startVUs: 10,
      stages: [
        { duration: '60s', target: 100 },
        { duration: '60s', target: 250 },
        { duration: '60s', target: 500 },
        { duration: '60s', target: 500 },  // Hold at peak
        { duration: '60s', target: 10 },    // Ramp down
      ],
    },
  },
  thresholds: {
    'success_rate': ['rate>0.70'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), true, 'medium', 0.05);
}
