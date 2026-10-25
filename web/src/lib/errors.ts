import { ApiError } from "./api";

type Translate = (key: string, vars?: Record<string, string | number>) => string;

export function apiErrorMessage(t: Translate, error: unknown): string {
  if (error instanceof ApiError) {
    const key = `errors.${error.code}`;
    const translated = t(key);
    return translated === key ? t("errors.unknown") : translated;
  }
  return t("errors.unknown");
}
