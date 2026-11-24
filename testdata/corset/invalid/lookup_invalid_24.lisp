;;error:3:12-14:malformed handle
(defcolumns (X :i16) (Y :i16))
(deflookup () ((_ X)) ((_ Y)))
