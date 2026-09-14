#!/usr/bin/env python3
"""Sync bd (beads) status into GitHub Project v2 #5 (misfitdev/frank-grimes).

Beads is the source of truth. Publishes beads that have no GitHub issue yet
and reconciles issue state + project Status for beads that do. Safe to run
repeatedly; never raises past a warning so it can't break a git hook.
"""

import json
import subprocess
import sys

REPO = "misfitdev/frank-grimes"
OWNER = "misfitdev"
PROJECT_NUMBER = "5"
PROJECT_ID = "PVT_kwDOBX3iRc4BjO3z"
STATUS_FIELD_ID = "PVTSSF_lADOBX3iRc4BjO3zzhiEAtM"

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


def get_beads():
    result = run(["bd", "list", "--format=json"])
    return json.loads(result.stdout)


def get_bead_detail(bead_id):
    result = run(["bd", "show", bead_id, "--format=json"])
    data = json.loads(result.stdout)
    return data[0] if isinstance(data, list) else data


def get_repo_labels():
    result = run(["gh", "label", "list", "--repo", REPO, "--limit", "200", "--json", "name"])
    return {label["name"] for label in json.loads(result.stdout)}


def get_project_items():
    result = run(["gh", "project", "item-list", PROJECT_NUMBER, "--owner", OWNER, "--format", "json"])
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
    if "/issues/" in external_ref:
        return external_ref.rsplit("/", 1)[-1]
    return None


def labels_for_bead(bead, repo_labels):
    labels = []
    type_label = TYPE_LABELS.get(bead.get("issue_type"))
    if type_label and type_label in repo_labels:
        labels.append(type_label)
    priority_label = PRIORITY_LABELS.get(bead.get("priority"))
    if priority_label and priority_label in repo_labels:
        labels.append(priority_label)
    return labels


def create_issue(bead, repo_labels):
    title = bead["title"]
    body = (bead.get("description") or "").strip()
    body += f"\n\n---\nSynced from beads: `{bead['id']}`"
    labels = labels_for_bead(bead, repo_labels)

    cmd = ["gh", "issue", "create", "--repo", REPO, "--title", title, "--body", body]
    for label in labels:
        cmd += ["--label", label]

    result = run(cmd)
    url = result.stdout.strip().splitlines()[-1]
    run(["bd", "update", bead["id"], f"--external-ref={url}"])
    return url


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
    if want_open:
        run(["gh", "issue", "reopen", issue_number, "--repo", REPO], check=False)
    else:
        run(["gh", "issue", "close", issue_number, "--repo", REPO], check=False)


def get_issue_state(issue_number):
    result = run(["gh", "issue", "view", issue_number, "--repo", REPO, "--json", "state"])
    return json.loads(result.stdout)["state"]


def main():
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
            try:
                issue_url = create_issue(bead, repo_labels)
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
            set_issue_state(issue_number, want_issue_state == "open")
            print(f"{bead_id}: issue #{issue_number} {current_issue_state} -> {want_issue_state}")
            changes += 1

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


if __name__ == "__main__":
    sys.exit(main())
