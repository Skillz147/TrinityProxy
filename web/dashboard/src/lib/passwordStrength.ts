export type PasswordStrength = "weak" | "fair" | "good" | "strong";

export interface PasswordStrengthResult {
  score: PasswordStrength;
  label: string;
  hints: string[];
  percent: number;
}

export function evaluatePasswordStrength(password: string): PasswordStrengthResult {
  const hints: string[] = [];
  let points = 0;

  if (password.length >= 8) points += 1;
  else hints.push("Use at least 8 characters");

  if (password.length >= 12) points += 1;

  if (/[a-z]/.test(password) && /[A-Z]/.test(password)) points += 1;
  else hints.push("Mix uppercase and lowercase letters");

  if (/\d/.test(password)) points += 1;
  else hints.push("Include at least one number");

  if (/[^A-Za-z0-9]/.test(password)) points += 1;
  else hints.push("Add a symbol for extra strength");

  let score: PasswordStrength;
  let label: string;
  let percent: number;

  if (points <= 1) {
    score = "weak";
    label = "Weak";
    percent = 25;
  } else if (points === 2) {
    score = "fair";
    label = "Fair";
    percent = 50;
  } else if (points === 3 || points === 4) {
    score = "good";
    label = "Good";
    percent = 75;
  } else {
    score = "strong";
    label = "Strong";
    percent = 100;
  }

  return { score, label, hints, percent };
}
