// Preset OpenAI-format request bodies for load testing
export const PRESETS = [
  {
    id: 1,
    name: 'short_qa',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a helpful assistant.' },
        { role: 'user', content: 'What is the capital of France?' },
      ],
      temperature: 0.7,
      max_tokens: 256,
    },
  },
  {
    id: 2,
    name: 'code_generation',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are an expert programmer. Write clean, efficient code.' },
        { role: 'user', content: 'Write a Go function that implements a concurrent-safe LRU cache with TTL support.' },
      ],
      temperature: 0.7,
      max_tokens: 2048,
    },
  },
  {
    id: 3,
    name: 'long_text',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a knowledgeable writer. Provide detailed, comprehensive responses.' },
        { role: 'user', content: 'Explain the history and evolution of computer networking from ARPANET to modern cloud computing.' },
      ],
      temperature: 0.7,
      max_tokens: 4096,
    },
  },
  {
    id: 4,
    name: 'tool_call',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are a helpful assistant with access to tools. Use tools when appropriate.' },
        { role: 'user', content: "What's the current weather in Tokyo and New York?" },
      ],
      temperature: 0.7,
      max_tokens: 1024,
      tools: [
        {
          type: 'function',
          function: {
            name: 'get_weather',
            description: 'Get the current weather for a location',
            parameters: {
              type: 'object',
              properties: {
                locations: { type: 'array', items: { type: 'string' }, description: 'List of city names' },
                units: { type: 'string', enum: ['celsius', 'fahrenheit'] },
              },
              required: ['locations'],
            },
          },
        },
      ],
    },
  },
  {
    id: 5,
    name: 'multi_turn',
    body: {
      model: 'gemini-2.0-flash',
      messages: [
        { role: 'system', content: 'You are an expert data analyst. Help users understand their data.' },
        { role: 'user', content: 'I have a dataset of 10,000 customer transactions. How should I start analyzing it?' },
        { role: 'assistant', content: 'Great question! Start with exploratory data analysis (EDA). First, examine the structure: check column types, missing values, and basic statistics using describe(). Then look at distributions of key variables like transaction amount and frequency.' },
        { role: 'user', content: "I found that 15% of records have missing values in the 'category' field. What should I do?" },
        { role: 'assistant', content: "15% is significant. Before deciding, investigate if the missingness is random (MCAR), depends on other variables (MAR), or is systematic (MNAR). Check if missing categories correlate with transaction amount or date." },
        { role: 'user', content: "The missing values seem random. I'll use mode imputation. Now I want to segment customers. What clustering approach do you recommend?" },
      ],
      temperature: 0.7,
      max_tokens: 2048,
    },
  },
];

export const LONG_TEXT_PRESET = PRESETS[2]; // For S9 scenario
