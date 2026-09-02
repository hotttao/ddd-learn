import { Namespace, Context, SubjectSet } from "@ory/keto-namespace-types"

// Kratos identity.id is used as the stable user identifier. The User
// namespace intentionally has no business relations of its own.
class User implements Namespace {}

// Organization permissions have two independent layers:
// 1. entitled_* is the operation ceiling owned by the organization.
// 2. granted_* is the subset assigned to roles inside the organization.
// An effective permission requires membership in both subject sets.
class Organization implements Namespace {
  related: {
    members: User[]
    admins: User[]

    entitled_start_crawl: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
    entitled_view_content: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
    entitled_modify_keywords: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]

    granted_start_crawl: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
    granted_view_content: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
    granted_modify_keywords: (SubjectSet<Organization, "members"> | SubjectSet<Organization, "admins">)[]
  }

  permits = {
    // The organization owns the operation and the user's role is granted it.
    start_crawl: (ctx: Context): boolean =>
      this.related.entitled_start_crawl.includes(ctx.subject) &&
      this.related.granted_start_crawl.includes(ctx.subject),

    view_content: (ctx: Context): boolean =>
      this.related.entitled_view_content.includes(ctx.subject) &&
      this.related.granted_view_content.includes(ctx.subject),

    modify_keywords: (ctx: Context): boolean =>
      this.related.entitled_modify_keywords.includes(ctx.subject) &&
      this.related.granted_modify_keywords.includes(ctx.subject),
  }
}
