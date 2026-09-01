import { Namespace, Context } from "@ory/keto-namespace-types"

// Kratos identity.id is used as the stable user identifier. The User
// namespace intentionally has no business relations of its own.
class User implements Namespace {}

// Organization stores role facts. Permissions are derived from these facts
// and are not written as separate relation tuples.
class Organization implements Namespace {
  related: {
    members: User[]
    admins: User[]
  }

  permits = {
    // Members and admins can start a crawl task.
    start_crawl: (ctx: Context): boolean =>
      this.related.members.includes(ctx.subject) ||
      this.related.admins.includes(ctx.subject),

    // Members and admins can view crawl content.
    view_content: (ctx: Context): boolean =>
      this.related.members.includes(ctx.subject) ||
      this.related.admins.includes(ctx.subject),

    // Only admins can change the organization's crawl keywords.
    modify_keywords: (ctx: Context): boolean =>
      this.related.admins.includes(ctx.subject),
  }
}
