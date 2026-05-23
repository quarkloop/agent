export const SUBJECTS = {
  spaceList: "control.space.v1.list",
  spaceCredential: "control.space.v1.credential",
  sessionCreate: "control.session.v1.create",
  sessionList: "control.session.v1.list",
  sessionDelete: "control.session.v1.delete",
  sessionCredential: "control.session.v1.credential",
  runtimeInfoGet: "runtime.info.v1.get",
  runtimePlanGet: "runtime.plan.v1.get",
  runtimePlanApprove: "runtime.plan.v1.approve",
  runtimePlanReject: "runtime.plan.v1.reject",
  runtimeActivityList: "runtime.activity.v1.list",
  runtimeActivityFeed: "runtime.activity.v1.events",
} as const;

const tokenPattern = /^[a-z][a-z0-9_]*$/;

export function sessionInputSubject(sessionID: string): string {
  return `session.${subjectToken("session_id", sessionID)}.input`;
}

export function sessionEventsSubject(sessionID: string): string {
  return `session.${subjectToken("session_id", sessionID)}.events`;
}

export function subjectToken(name: string, value: string): string {
  const token = stableToken(value);
  if (!token) throw new Error(`${name} is required`);
  if (!tokenPattern.test(token)) throw new Error(`${name} ${token} is invalid`);
  return token;
}

export function stableToken(value: string): string {
  let out = "";
  let lastUnderscore = false;
  let prevLowerOrDigit = false;
  for (const char of value.trim()) {
    if (/[A-Za-z0-9]/.test(char)) {
      const isUpper = char >= "A" && char <= "Z";
      if (isUpper && prevLowerOrDigit && !lastUnderscore && out.length > 0) {
        out += "_";
      }
      out += char.toLowerCase();
      lastUnderscore = false;
      prevLowerOrDigit = /[a-z0-9]/.test(char);
      continue;
    }
    if (char === "_" || char === "-" || char === ".") {
      if (!lastUnderscore && out.length > 0) {
        out += "_";
        lastUnderscore = true;
      }
      prevLowerOrDigit = false;
    }
  }
  out = out.replace(/^_+|_+$/g, "");
  if (/^[0-9]/.test(out)) out = `id_${out}`;
  return out;
}
