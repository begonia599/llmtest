// S7: Mixed realistic scenario
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s7: {
      executor: 'constant-vus',
      vus: 200,
      duration: '180s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.85'],
  },
};

export default function () {
  const stream = Math.random() < 0.7; // 70% streaming
  makeRequest(pickRandom(PRESETS), stream, 'realistic', 0.05);
}
