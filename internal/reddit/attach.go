package reddit

// Attach splices the result of a MoreChildren call for stub into t.
//
// It removes stub from its parent, attaches every returned comment whose
// parent is loaded or in the response (two passes, so order does not
// matter), attaches returned stubs, and re-adds a stub for any of stub's IDs
// the response did not cover. It returns the number of comments attached.
func Attach(t *Thread, stub *MoreStub, th Things) int {
	index := map[string]*Comment{}
	var walk func([]*Comment)
	walk = func(cs []*Comment) {
		for _, c := range cs {
			index[c.Fullname] = c
			walk(c.Children)
		}
	}
	walk(t.Comments)

	postName := ""
	if t.Post != nil {
		postName = t.Post.Fullname
	}
	setStub := func(parent string, s *MoreStub) {
		if parent == postName {
			t.More = s
		} else if p, ok := index[parent]; ok {
			p.More = s
		}
	}
	if stub != nil {
		setStub(stub.ParentFullname, nil)
	}

	returned := map[string]bool{}
	pending := append([]*Comment(nil), th.Comments...)
	attached := 0
	for progress := true; progress && len(pending) > 0; {
		progress = false
		var rest []*Comment
		for _, c := range pending {
			returned[c.ID] = true
			if _, dup := index[c.Fullname]; dup {
				progress = true
				continue
			}
			switch p, ok := index[c.ParentFullname]; {
			case c.ParentFullname == postName:
				c.Depth = 0
				t.Comments = append(t.Comments, c)
			case ok:
				c.Depth = p.Depth + 1
				p.Children = append(p.Children, c)
			default:
				rest = append(rest, c)
				continue
			}
			index[c.Fullname] = c
			attached++
			progress = true
		}
		pending = rest
	}

	for _, s := range th.Stubs {
		for _, id := range s.IDs {
			returned[id] = true
		}
		setStub(s.ParentFullname, mergeStubs(currentStub(t, index, s.ParentFullname), s))
	}

	if stub != nil {
		var remaining []string
		for _, id := range stub.IDs {
			if !returned[id] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) > 0 {
			ns := &MoreStub{ParentFullname: stub.ParentFullname, Count: len(remaining), IDs: remaining}
			setStub(stub.ParentFullname, mergeStubs(currentStub(t, index, stub.ParentFullname), ns))
		}
	}
	return attached
}

func currentStub(t *Thread, index map[string]*Comment, parent string) *MoreStub {
	if t.Post != nil && parent == t.Post.Fullname {
		return t.More
	}
	if p, ok := index[parent]; ok {
		return p.More
	}
	return nil
}
