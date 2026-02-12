// S9: Long text generation streaming
import { makeRequest } from '../helpers.js';
import { LONG_TEXT_PRESET } from '../presets.js';

export const options = {
  scenarios: {
    s9: {
      executor: 'constant-vus',
      vus: 50,
      duration: '180s',
    },
  },
  thresholds: {
    'success_rate': ['rate>0.90'],
  },
};

export default function () {
  makeRequest(LONG_TEXT_PRESET, true, 'slow', 0);
}
