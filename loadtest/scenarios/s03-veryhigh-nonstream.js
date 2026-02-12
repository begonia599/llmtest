// S3: Very high concurrency non-streaming
import { makeRequest, pickRandom } from '../helpers.js';
import { PRESETS } from '../presets.js';

export const options = {
  scenarios: {
    s3: {
      executor: 'constant-vus',
      vus: 500,
      duration: '60s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.80'],
  },
};

export default function () {
  makeRequest(pickRandom(PRESETS), false, 'fast', 0);
}
