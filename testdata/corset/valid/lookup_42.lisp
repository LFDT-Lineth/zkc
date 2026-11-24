;; As lookup_11, but without selectors.
(deflookup test ((_ m1.X)) ((_ m1.Y)))
(module m1)
(defcolumns (X :i16) (Y :i16))
