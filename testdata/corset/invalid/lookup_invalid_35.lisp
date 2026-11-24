;;error:7:12-16:duplicate handle
(module m1)
(defcolumns (X :i16) (Y :i16))
(deflookup test ((_ X)) ((_ Y)))

(module m1)
(deflookup test ((_ Y)) ((_ X)))
