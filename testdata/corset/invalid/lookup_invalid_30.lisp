;;error:3:32-39:malformed source column
(defcolumns (X :u16) (Y :u16))
(deflookup (l1 :unchecked) ((_ (- 1 Y))) ((_ X)))
