#!/usr/bin/env python3
"""Sync bd (beads) status into GitHub Project v2 #5 (misfitdev/frank-grimes).

Beads is the source of truth. Publishes beads that have no GitHub issue yet
and reconciles issue state + project Status for beads that do. Safe to run
repeatedly; never raises past a warning so it can't break a git hook.
"""

import fcntl
import json
import os
import re
import subprocess
import sys

REPO = "misfitdev/frank-grimes"
OWNER = "misfitdev"
PROJECT_NUMBER = "5"
PROJECT_ID = "PVT_kwDOBX3iRc4BjO3z"
STATUS_FIELD_ID = "PVTSSF_lADOBX3iRc4BjO3zzhiEAtM"
PROJECT_ITEM_LIMIT = 1000
ISSUE_REF_RE = re.compile(
    r"^https://github\.com/" + re.escape(REPO) + r"/issues/(\d+)$"
)

STATUS_OPTION_IDS = {
    "Backlog": "f75ad846",
    "Ready": "08afe404",
    "Blocked": "d6815222",
    "In progress": "47fc9ee4",
    "In review": "4cc61d42",
    "Done": "98236657",
}

# bd status -> (github issue state, project Status option)
BD_STATUS_MAP = {
    "open": ("open", "Backlog"),
    "in_progress": ("open", "In progress"),
    "blocked": ("open", "Blocked"),
    "deferred": ("open", "Backlog"),
    "closed": ("closed", "Done"),
}

TYPE_LABELS = {
    "bug": "type::bug",
    "feature": "type::feature",
    "task": "type::task",
    "epic": "type::epic",
}

PRIORITY_LABELS = {
    0: "priority::critical",
    1: "priority::high",
    2: "priority::medium",
    3: "priority::low",
    4: "priority::low",
}


def run(cmd, check=True, input_text=None):
    result = subprocess.run(
        cmd, capture_output=True, text=True, input=input_text
    )
    if check and result.returncode != 0:
        raise RuntimeError(f"{' '.join(cmd)} failed: {result.stderr.strip()}")
    return result


def warn(msg):
    print(f"sync-beads-github: {msg}", file=sys.stderr)


def acquire_lock():
    """Non-blocking lock so concurrent hook/manual runs can't race on the
    same bead and publish duplicate GitHub issues."""
    repo_root = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True
    ).stdout.strip()
    lock_path = os.path.join(repo_root, ".beads", "sync-github.lock")
    lock_file = open(lock_path, "w")
    try:
        fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        lock_file.close()
        return None
    return lock_file


def get_beads():
    result = run(["bd", "list", "--all", "--format=json"])
    return json.loads(result.stdout)


def get_bead_detail(bead_id):
    result = run(["bd", "show", bead_id, "--format=json"])
    data = json.loads(result.stdout)
    return data[0] if isinstance(data, list) else data


def get_repo_labels():
    result = run(["gh", "label", "list", "--repo", REPO, "--limit", "200", "--json", "name"])
    return {label["name"] for label in json.loads(result.stdout)}


def get_project_items():
    result = run([
        "gh", "project", "item-list", PROJECT_NUMBER,
        "--owner", OWNER, "--limit", str(PROJECT_ITEM_LIMIT), "--format", "json",
    ])
    data = json.loads(result.stdout)
    items = {}
    for item in data["items"]:
        content = item.get("content", {})
        if content.get("type") == "Issue":
            items[str(content["number"])] = item
    return items


def issue_number_from_ref(external_ref):
    if not external_ref:
        return None
    match = ISSUE_REF_RE.match(external_ref.strip())
    return match.group(1) if match else None


def labels_for_bead(bead, repo_labels):
    labels = []
    type_label = TYPE_LABELS.get(bead.get("issue_type"))
    if type_label and type_label in repo_labels:
        labels.append(type_label)
    priority_label = PRIORITY_LABELS.get(bead.get("priority"))
    if priority_label and priority_label in repo_labels:
        labels.append(priority_label)
    return labels


def synced_marker(bead_id):
    return f"Synced from beads: `{bead_id}`"


def find_existing_issue(bead_id):
    """Search for an issue already published for this bead, so a bead whose
    `bd update --external-ref` failed after a prior create doesn't get a
    second GitHub issue on retry.

    Raises on a search failure rather than returning None, so the caller
    can't mistake "the search itself failed" for "no issue exists" and
    create a duplicate.
    """
    marker = synced_marker(bead_id)
    result = run([
        "gh", "issue", "list", "--repo", REPO, "--state", "all",
        "--search", marker, "--json", "number,body", "--limit", "20",
    ])
    try:
        candidates = json.loads(result.stdout)
    except ValueError as exc:
        raise RuntimeError(f"malformed response searching for existing issue: {exc}")
    for candidate in candidates:
        if marker in (candidate.get("body") or ""):
            return str(candidate["number"])
    return None


def link_bead(bead_id, issue_url, attempts=3):
    for _ in range(attempts):
        result = run(["bd", "update", bead_id, f"--external-ref={issue_url}"], check=False)
        if result.returncode == 0:
            return True
    return False


def publish_bead(bead, repo_labels):
    bead_id = bead["id"]
    existing_number = find_existing_issue(bead_id)
    if existing_number:
        issue_url = f"https://github.com/{REPO}/issues/{existing_number}"
        warn(f"{bead_id}: found existing issue #{existing_number} via search, linking instead of creating a duplicate")
    else:
        title = bead["title"]
        body = (bead.get("description") or "").strip()
        body += f"\n\n---\n{synced_marker(bead_id)}"
        labels = labels_for_bead(bead, repo_labels)

        cmd = ["gh", "issue", "create", "--repo", REPO, "--title", title, "--body", body]
        for label in labels:
            cmd += ["--label", label]

        result = run(cmd)
        issue_url = result.stdout.strip().splitlines()[-1]

    if not link_bead(bead_id, issue_url):
        warn(f"{bead_id}: issue {issue_url} exists but writing external-ref back to bd failed; will retry next run")

    return issue_url


def add_to_project(issue_url):
    result = run(["gh", "project", "item-add", PROJECT_NUMBER, "--owner", OWNER, "--url", issue_url, "--format", "json"])
    return json.loads(result.stdout)["id"]


def set_project_status(item_id, status_name):
    option_id = STATUS_OPTION_IDS[status_name]
    run([
        "gh", "project", "item-edit",
        "--id", item_id,
        "--project-id", PROJECT_ID,
        "--field-id", STATUS_FIELD_ID,
        "--single-select-option-id", option_id,
    ])


def set_issue_state(issue_number, want_open):
    cmd = ["gh", "issue", "reopen" if want_open else "close", issue_number, "--repo", REPO]
    result = run(cmd, check=False)
    return result.returncode == 0


def get_issue_state(issue_number):
    result = run(["gh", "issue", "view", issue_number, "--repo", REPO, "--json", "state"])
    return json.loads(result.stdout)["state"]


def sync():
    try:
        repo_labels = get_repo_labels()
        project_items = get_project_items()
        beads = get_beads()
    except RuntimeError as exc:
        warn(f"skipping sync, setup failed: {exc}")
        return 0

    changes = 0

    for bead_summary in beads:
        bead_id = bead_summary["id"]
        try:
            bead = get_bead_detail(bead_id)
        except RuntimeError as exc:
            warn(f"{bead_id}: could not read detail, skipping ({exc})")
            continue

        external_ref = bead.get("external_ref")
        issue_number = issue_number_from_ref(external_ref)

        if not issue_number:
            if external_ref:
                warn(f"{bead_id}: external_ref '{external_ref}' does not point to a {REPO} issue, skipping")
                continue
            if bead.get("status") == "closed":
                # Never published and already resolved: publishing now would
                # only create an issue for the sole purpose of closing it.
                continue
            try:
                issue_url = publish_bead(bead, repo_labels)
                item_id = add_to_project(issue_url)
                issue_number = issue_url.rsplit("/", 1)[-1]
                project_items[issue_number] = {"id": item_id, "status": "Backlog"}
                print(f"{bead_id}: published as {issue_url}")
                changes += 1
            except RuntimeError as exc:
                warn(f"{bead_id}: failed to publish ({exc})")
                continue

        item = project_items.get(issue_number)
        if item is None:
            try:
                issue_url = f"https://github.com/{REPO}/issues/{issue_number}"
                item_id = add_to_project(issue_url)
                item = {"id": item_id, "status": "Backlog"}
                print(f"{bead_id}: re-added issue #{issue_number} to project")
                changes += 1
            except RuntimeError as exc:
                warn(f"{bead_id}: could not add issue #{issue_number} to project ({exc})")
                continue

        bd_status = bead.get("status")
        mapped = BD_STATUS_MAP.get(bd_status)
        if not mapped:
            warn(f"{bead_id}: unknown bd status '{bd_status}', skipping")
            continue
        want_issue_state, want_project_status = mapped

        try:
            current_issue_state = get_issue_state(issue_number).lower()
        except RuntimeError as exc:
            warn(f"{bead_id}: could not read issue #{issue_number} state ({exc})")
            continue

        if current_issue_state != want_issue_state:
            if set_issue_state(issue_number, want_issue_state == "open"):
                print(f"{bead_id}: issue #{issue_number} {current_issue_state} -> {want_issue_state}")
                changes += 1
            else:
                warn(f"{bead_id}: failed to transition issue #{issue_number} to {want_issue_state}, skipping status update")
                continue

        current_status = item.get("status")
        if current_status != want_project_status:
            try:
                set_project_status(item["id"], want_project_status)
                print(f"{bead_id}: {current_status} -> {want_project_status}")
                changes += 1
            except RuntimeError as exc:
                warn(f"{bead_id}: failed to update project status ({exc})")

    if changes == 0:
        print("sync-beads-github: no changes")

    return 0


def main():
    lock = acquire_lock()
    if lock is None:
        warn("another sync is already running, skipping")
        return 0
    try:
        return sync()
    finally:
        fcntl.flock(lock, fcntl.LOCK_UN)
        lock.close()


if __name__ == "__main__":
    sys.exit(main())
