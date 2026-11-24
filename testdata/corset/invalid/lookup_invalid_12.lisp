;;error:3:34-41:malformed source column
(defcolumns (X :u16) (Y :u16) (one :i1 :constant 1))
(deflookup (l1 :unchecked) ((one (- 1 Y))) ((one X)))
