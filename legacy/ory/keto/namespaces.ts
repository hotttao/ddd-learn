// Keto relation model for the playground. Compile/load this with the Keto
// namespace manager used by the selected Keto image version.
namespace XhsWorkspace {
  relation member: User
  relation editor: User
  permission read = member + editor
  permission write = editor
}

