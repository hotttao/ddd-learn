namespace XhsWorkspace {
  relation member: User
  relation editor: User
  permission read = member + editor
  permission write = editor
}

