import { GoogleGenerativeAI } from '@google/generative-ai';

/**
 * Result of Gemini API key validation.
 */
export interface GeminiAuthResult {
  /** Whether the API key is valid */
  valid: boolean;
  /** Model name that was successfully accessed */
  model?: string;
  /** Error message if validation failed */
  error?: string;
}

/**
 * The model to use for validation.
 * Using gemini-1.5-flash as it's widely available and fast.
 */
const VALIDATION_MODEL = 'gemini-1.5-flash';

/**
 * Validate a Google Gemini API key.
 *
 * Makes a minimal API call to verify the key is valid and has access
 * to the Gemini API. Does not throw - returns structured result.
 *
 * @param apiKey - The Gemini API key to validate
 * @returns Validation result indicating success or failure
 */
export async function validateGeminiKey(apiKey: string): Promise<GeminiAuthResult> {
  if (!apiKey || apiKey.trim() === '') {
    return {
      valid: false,
      error: 'API key is empty',
    };
  }

  try {
    const genAI = new GoogleGenerativeAI(apiKey);
    const model = genAI.getGenerativeModel({ model: VALIDATION_MODEL });

    // Make a minimal request to validate the key
    // Using countTokens as it's fast and doesn't consume quota
    await model.countTokens('test');

    return {
      valid: true,
      model: VALIDATION_MODEL,
    };
  } catch (error) {
    if (error instanceof Error) {
      const message = error.message.toLowerCase();

      // Check for authentication errors
      if (message.includes('api key') || message.includes('invalid') || message.includes('401')) {
        return {
          valid: false,
          error: 'Invalid API key',
        };
      }

      // Check for quota/rate limit errors (key is valid but limited)
      if (message.includes('quota') || message.includes('rate limit') || message.includes('429')) {
        return {
          valid: true,
          model: VALIDATION_MODEL,
          error: 'API key is valid but rate limited or quota exceeded',
        };
      }

      // Check for permission errors
      if (message.includes('permission') || message.includes('403')) {
        return {
          valid: false,
          error: 'API key lacks required permissions',
        };
      }

      return {
        valid: false,
        error: `Failed to validate API key: ${error.message}`,
      };
    }

    return {
      valid: false,
      error: 'Unknown error validating API key',
    };
  }
}

/**
 * Get the model name used for validation.
 *
 * @returns The Gemini model name used for API key validation
 */
export function getValidationModel(): string {
  return VALIDATION_MODEL;
}
