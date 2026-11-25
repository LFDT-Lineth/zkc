;;error:3:21-22:unknown symbol
(defcolumns (X :i16))
(deflookup test ((_ Y)) ((_ X)))

(module m1)
(defcolumns (Y :i16))
