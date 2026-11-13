package kinds

// middlewareMembers holds one optional Middleware spec member per public
// tenant type with a configuration schema, named after the type as
// tenanttype.Type.Member returns it. Each member mirrors the type's JSON
// Schema structurally: the same properties, JSON types, array items,
// required fields and descriptions, with every object closed. It declares
// no value constraints, such as enums, patterns or bounds, and no defaults;
// admission checks those against the type's schema. A manifestdata test
// fails when a member and its type disagree.
middlewareMembers: {}
